// SPDX-License-Identifier: GPL-2.0-or-later
using System.Collections.Concurrent;
using System.Security.Cryptography;
using Jellyfin.Api.Extensions;
using Jellyfin.Server.Implementations.SystemBackupService;
using MediaBrowser.Common.Api;
using MediaBrowser.Common.Configuration;
using MediaBrowser.Controller.SystemBackupService;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;

namespace Mutti.Export;

// Installing this helper grants no download link. Each export requires a new,
// elevated admin session and is bound to that user, device and one-use secret.
[ApiController]
[Authorize(Policy = Policies.RequiresElevation)]
[Route("MuttiExport")]
public sealed class ExportController(IBackupService backups, IApplicationPaths paths) : ControllerBase
{
    private sealed record Job(Guid User, string Device, byte[] Secret, string Path, DateTime Expires);
    private static readonly ConcurrentDictionary<string, Job> Jobs = new();
    private static readonly SemaphoreSlim Gate = new(1, 1);
    // Expired capabilities are removed even if the recipient disappears. The
    // normal Jellyfin backup remains under Jellyfin's own backup retention.
    private static readonly Timer Expiry = new(_ => Expire(), null, TimeSpan.FromMinutes(1), TimeSpan.FromMinutes(1));
    public sealed record BeginRequest(string Recipient);
    public sealed record Ticket(string Id, string Secret);
    private static void Expire()
    {
        foreach (var (id, job) in Jobs)
            if (job.Expires <= DateTime.UtcNow) Jobs.TryRemove(id, out _);
    }
    private bool SecureTransport() => Request.IsHttps ||
        (HttpContext.Connection.RemoteIpAddress is { } ip && System.Net.IPAddress.IsLoopback(ip));

    [HttpPost("Begin")]
    public async Task<ActionResult<Ticket>> Begin(BeginRequest request)
    {
        if (!SecureTransport()) return BadRequest("HTTPS is required outside this host.");
        var device = User.GetDeviceId();
        if (string.IsNullOrEmpty(device) || request.Recipient != device || !device.StartsWith("mutti-import-", StringComparison.Ordinal)) return BadRequest();
        Expire();
        if (!await Gate.WaitAsync(0, HttpContext.RequestAborted)) return Conflict("An export is already being prepared.");
        try
        {
            if (Jobs.Count >= 4) return Conflict("Finish the previous export first.");
            var manifest = await backups.CreateBackupAsync(new BackupOptionsDto { Database = true, Metadata = true, Subtitles = true, Trickplay = true });
            var archive = Path.GetFullPath(manifest.Path);
            var folder = Path.GetFullPath(paths.BackupPath) + Path.DirectorySeparatorChar;
            if (!archive.StartsWith(folder, StringComparison.Ordinal) || !System.IO.File.Exists(archive)) return StatusCode(500);
            if (!OperatingSystem.IsWindows()) System.IO.File.SetUnixFileMode(archive, UnixFileMode.UserRead | UnixFileMode.UserWrite);
            if (HttpContext.RequestAborted.IsCancellationRequested) return StatusCode(499);
            var id = Convert.ToHexStringLower(RandomNumberGenerator.GetBytes(24));
            var secret = RandomNumberGenerator.GetBytes(32);
            Jobs[id] = new(User.GetUserId(), device, secret, archive, DateTime.UtcNow.AddMinutes(30));
            Response.Headers.CacheControl = "no-store";
            return new Ticket(id, Convert.ToHexStringLower(secret));
        }
        finally { Gate.Release(); }
    }
    private bool Authorized(Ticket ticket, out Job? job)
    {
        job = null;
        if (!SecureTransport() || ticket.Id is null || ticket.Secret is null || ticket.Id.Length != 48 || ticket.Secret.Length != 64) return false;
        if (!Jobs.TryGetValue(ticket.Id, out var candidate) || candidate.Expires <= DateTime.UtcNow || candidate.User != User.GetUserId() || candidate.Device != User.GetDeviceId()) return false;
        byte[] secret;
        try { secret = Convert.FromHexString(ticket.Secret); } catch (FormatException) { return false; }
        if (!CryptographicOperations.FixedTimeEquals(secret, candidate.Secret)) return false;
        job = candidate; return true;
    }
    [HttpPost("Download")]
    public IActionResult Download(Ticket ticket)
    {
        if (!Authorized(ticket, out var job)) return NotFound();
        // Consume before streaming: interrupted downloads need a new request.
        if (!Jobs.TryRemove(ticket.Id, out _)) return NotFound();
        Response.Headers.CacheControl = "no-store";
        Response.Headers["X-Content-Type-Options"] = "nosniff";
        return PhysicalFile(job!.Path, "application/zip", "mutti-transfer.zip", enableRangeProcessing: false);
    }
    [HttpPost("Finish")]
    public IActionResult Finish(Ticket ticket)
    {
        if (Authorized(ticket, out _)) Jobs.TryRemove(ticket.Id, out _);
        return NoContent();
    }
}
