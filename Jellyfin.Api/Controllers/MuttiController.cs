// SPDX-License-Identifier: GPL-2.0-or-later
using System;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using Jellyfin.Api.Extensions;
using MediaBrowser.Common.Api;
using Microsoft.AspNetCore.Authorization;
using Microsoft.AspNetCore.Http;
using Microsoft.AspNetCore.Mvc;

namespace Jellyfin.Api.Controllers;

/// <summary>Local server-management integration for the Mutti shell.</summary>
[Authorize(Policy = Policies.RequiresElevation)]
[Route("Mutti")]
public class MuttiController : BaseJellyfinApiController
{
    private static readonly HttpClient ConnectClient = new(new HttpClientHandler { AllowAutoRedirect = false, UseProxy = false })
    {
        Timeout = TimeSpan.FromSeconds(12)
    };

    /// <summary>Gets package-owned management capabilities.</summary>
    /// <returns>The configured onboarding location and Connect availability.</returns>
    [HttpGet("Management")]
    public ActionResult GetManagement()
    {
        var onboarding = Environment.GetEnvironmentVariable("MUTTI_MANAGEMENT_ORIGIN");
        var valid = Uri.TryCreate(onboarding, UriKind.Absolute, out var origin)
            && (origin.Scheme == "http" || origin.Scheme == "https")
            && string.IsNullOrEmpty(origin.UserInfo) && string.IsNullOrEmpty(origin.Query)
            && string.IsNullOrEmpty(origin.Fragment) && origin.AbsolutePath == "/";
        return new OkObjectResult(new { onboardingUrl = valid ? onboarding!.TrimEnd('/') + "/#import" : null, connectAvailable = ConnectTarget() is not null });
    }

    /// <summary>Forwards an allowlisted owner action to this package's local Connect service.</summary>
    /// <param name="operation">An explicitly supported Connect action.</param>
    /// <param name="body">The action's JSON payload.</param>
    /// <param name="cancellationToken">Request cancellation.</param>
    /// <returns>The local Connect result, without creating another identity or credential.</returns>
    [HttpPost("Connect/{operation}")]
    [RequestSizeLimit(16384)]
    public async Task<ActionResult> Connect(string operation, [FromBody] JsonElement body, CancellationToken cancellationToken)
    {
        if (operation is not ("state" or "invite" or "qr" or "approve" or "revoke" or "profile"))
        {
            return NotFound();
        }

        var target = ConnectTarget();
        if (target is null)
        {
            return StatusCode(StatusCodes.Status503ServiceUnavailable, "Die Gerätekopplung ist in diesem Paket nicht bereit.");
        }

        var token = User.GetToken();
        if (string.IsNullOrEmpty(token))
        {
            return Unauthorized();
        }

        // The browser can never supply a target address or a route outside this list.
        using var request = new HttpRequestMessage(HttpMethod.Post, new Uri(target, operation));
        request.Headers.Host = new Uri(Environment.GetEnvironmentVariable("MUTTI_CONNECT_ADMIN_ORIGIN")!).Authority;
        request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
        request.Content = new StringContent(body.GetRawText(), Encoding.UTF8, "application/json");
        try
        {
            using var response = await ConnectClient.SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
            // Small JSON state or a QR PNG only. Do not turn this into an arbitrary proxy.
            await response.Content.LoadIntoBufferAsync(2 * 1024 * 1024, cancellationToken).ConfigureAwait(false);
            var bytes = await response.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false);
            Response.Headers.CacheControl = "no-store";
            Response.StatusCode = (int)response.StatusCode;
            if (response.StatusCode == System.Net.HttpStatusCode.NoContent)
            {
                return NoContent();
            }

            var type = operation == "qr" && response.IsSuccessStatusCode ? "image/png" : response.Content.Headers.ContentType?.MediaType == "application/json" ? "application/json" : "text/plain; charset=utf-8";
            return File(bytes, type);
        }
        catch (Exception ex) when (ex is HttpRequestException or TaskCanceledException)
        {
            return StatusCode(StatusCodes.Status502BadGateway, "Die Gerätekopplung ist gerade nicht erreichbar. Bitte erneut versuchen.");
        }
    }

    private static Uri? ConnectTarget()
    {
        var host = Environment.GetEnvironmentVariable("MUTTI_CONNECT_ADMIN_ORIGIN");
        if (!Uri.TryCreate(host, UriKind.Absolute, out var origin)
            || (origin.Scheme != "http" && origin.Scheme != "https")
            || !string.IsNullOrEmpty(origin.UserInfo) || !string.IsNullOrEmpty(origin.Query)
            || !string.IsNullOrEmpty(origin.Fragment) || origin.AbsolutePath != "/"
            || !int.TryParse(Environment.GetEnvironmentVariable("MUTTI_CONNECT_ADMIN_PORT"), out var port) || port < 1 || port > 65535)
        {
            return null;
        }

        return new Uri($"http://127.0.0.1:{port}/");
    }
}
