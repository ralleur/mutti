// SPDX-License-Identifier: GPL-2.0-or-later
using System;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
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
public partial class MuttiController : BaseJellyfinApiController
{
    private static readonly HttpClient ConnectClient = new(new HttpClientHandler { AllowAutoRedirect = false, UseProxy = false })
    {
        Timeout = TimeSpan.FromSeconds(12)
    };

    // Starting the local AI engine or verifying a model may take longer.
    private static readonly HttpClient HubClient = new(new HttpClientHandler { AllowAutoRedirect = false, UseProxy = false })
    {
        Timeout = TimeSpan.FromSeconds(70)
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
        var hubAvailable = LocalTarget(onboarding, "MUTTI_HUB_PORT", "/") is not null;
        return new OkObjectResult(new { onboardingUrl = valid ? onboarding!.TrimEnd('/') + "/#import" : null, connectAvailable = ConnectTarget() is not null, modulesAvailable = hubAvailable });
    }

    /// <summary>Forwards an allowlisted owner action to this package's local Connect service.</summary>
    /// <param name="operation">An explicitly supported Connect action.</param>
    /// <param name="cancellationToken">Request cancellation.</param>
    /// <returns>The local Connect result, without creating another identity or credential.</returns>
    [HttpPost("Connect/{operation}")]
    [Consumes("application/json")]
    [RequestSizeLimit(16384)]
    public async Task<ActionResult> Connect(string operation, CancellationToken cancellationToken)
    {
        if (operation is not ("state" or "invite" or "qr" or "approve" or "revoke" or "profile"))
        {
            return NotFound();
        }

        return await Forward(operation, ConnectTarget(), Environment.GetEnvironmentVariable("MUTTI_CONNECT_ADMIN_ORIGIN"), cancellationToken).ConfigureAwait(false);
    }

    /// <summary>Performs a bounded owner maintenance operation on the package manager.</summary>
    /// <param name="operation">State, backup, verification or restore.</param>
    /// <param name="cancellationToken">Request cancellation.</param>
    /// <returns>The manager response.</returns>
    [HttpPost("Maintenance/{operation}")]
    [Consumes("application/json")]
    [RequestSizeLimit(16384)]
    public async Task<ActionResult> Maintenance(string operation, CancellationToken cancellationToken)
    {
        if (operation is not ("state" or "backup" or "verify" or "restore"))
        {
            return NotFound();
        }

        var origin = Environment.GetEnvironmentVariable("MUTTI_MANAGEMENT_ORIGIN");
        var target = LocalTarget(origin, "MUTTI_MANAGER_PORT", "/api/maintenance/");
        return await Forward(operation, target, origin, cancellationToken).ConfigureAwait(false);
    }

    /// <summary>Performs an allowlisted owner action on this package's module service.</summary>
    /// <param name="operation">A module administration route such as <c>admin/state</c>.</param>
    /// <param name="cancellationToken">Request cancellation.</param>
    /// <returns>The module service response.</returns>
    [HttpPost("Hub/{**operation}")]
    [Consumes("application/json")]
    [RequestSizeLimit(16384)]
    public async Task<ActionResult> Hub(string operation, CancellationToken cancellationToken)
    {
        if (!HubOperation().IsMatch(operation ?? string.Empty))
        {
            return NotFound();
        }

        var origin = Environment.GetEnvironmentVariable("MUTTI_MANAGEMENT_ORIGIN");
        var target = LocalTarget(origin, "MUTTI_HUB_PORT", "/mutti/hub/v1/");
        var method = operation == "admin/state" ? HttpMethod.Get : HttpMethod.Post;
        return await Forward(operation!, target, origin, cancellationToken, method, HubClient).ConfigureAwait(false);
    }

    [GeneratedRegex("^admin/(state|grants|ai/engine|enable/(ai|photos|documents)|(service|link|unlink)/(photos|documents)|ai/models/(pull|adopt|select|remove))$")]
    private static partial Regex HubOperation();

    private async Task<ActionResult> Forward(string operation, Uri? target, string? origin, CancellationToken cancellationToken, HttpMethod? method = null, HttpClient? client = null)
    {
        if (target is null)
        {
            return StatusCode(StatusCodes.Status503ServiceUnavailable, "Diese Verwaltungsfunktion ist in diesem Paket nicht bereit.");
        }

        var token = User.GetToken();
        if (string.IsNullOrEmpty(token))
        {
            return Unauthorized();
        }

        if (Request.ContentLength > 16384)
        {
            return StatusCode(StatusCodes.Status413PayloadTooLarge);
        }

        try
        {
            // Bound chunked bodies too, before parsing JSON or contacting Connect.
            var buffer = new byte[16385];
            var length = 0;
            while (length < buffer.Length)
            {
                var read = await Request.Body.ReadAsync(buffer.AsMemory(length), cancellationToken).ConfigureAwait(false);
                if (read == 0)
                {
                    break;
                }

                length += read;
            }

            if (length > 16384)
            {
                return StatusCode(StatusCodes.Status413PayloadTooLarge);
            }

            using var body = JsonDocument.Parse(buffer.AsMemory(0, length));
            // The browser can never supply a target address or a route outside this list.
            using var request = new HttpRequestMessage(method ?? HttpMethod.Post, new Uri(target, operation));
            request.Headers.Host = new Uri(origin!).Authority;
            request.Headers.Authorization = new AuthenticationHeaderValue("Bearer", token);
            if (request.Method != HttpMethod.Get)
            {
                request.Content = new StringContent(body.RootElement.GetRawText(), Encoding.UTF8, "application/json");
            }

            using var response = await (client ?? ConnectClient).SendAsync(request, HttpCompletionOption.ResponseHeadersRead, cancellationToken).ConfigureAwait(false);
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
        catch (BadHttpRequestException ex) when (ex.StatusCode == StatusCodes.Status413PayloadTooLarge)
        {
            return StatusCode(StatusCodes.Status413PayloadTooLarge);
        }
        catch (JsonException)
        {
            return BadRequest("Ungültige Anfrage.");
        }
        catch (Exception ex) when (ex is HttpRequestException or TaskCanceledException)
        {
            return StatusCode(StatusCodes.Status502BadGateway, "Die Verwaltung ist gerade nicht erreichbar. Bei einer Wiederherstellung bitte den Neustart abwarten und erneut anmelden.");
        }
    }

    private static Uri? ConnectTarget()
    {
        return LocalTarget(Environment.GetEnvironmentVariable("MUTTI_CONNECT_ADMIN_ORIGIN"), "MUTTI_CONNECT_ADMIN_PORT", "/");
    }

    private static Uri? LocalTarget(string? host, string portVariable, string path)
    {
        if (!Uri.TryCreate(host, UriKind.Absolute, out var origin)
            || (origin.Scheme != "http" && origin.Scheme != "https")
            || !string.IsNullOrEmpty(origin.UserInfo) || !string.IsNullOrEmpty(origin.Query)
            || !string.IsNullOrEmpty(origin.Fragment) || origin.AbsolutePath != "/"
            || !int.TryParse(Environment.GetEnvironmentVariable(portVariable), out var port) || port < 1 || port > 65535)
        {
            return null;
        }

        return new Uri($"http://127.0.0.1:{port}{path}");
    }
}
