// SPDX-License-Identifier: GPL-2.0-or-later
using System;
using System.Net;
using System.Threading.Tasks;
using Microsoft.AspNetCore.Http;

namespace Jellyfin.Server.Mutti;

/// <summary>Limits the private Mutti preview to its configured local origin.</summary>
public sealed class PreviewBoundaryMiddleware
{
    private readonly RequestDelegate _next;
    private readonly Uri _origin;
    private readonly bool _loopbackOnly;

    /// <summary>Initializes a new instance of the <see cref="PreviewBoundaryMiddleware"/> class.</summary>
    /// <param name="next">The next handler.</param>
    /// <param name="origin">The exact local browser origin.</param>
    /// <param name="loopbackOnly">Whether the socket peer must also be loopback.</param>
    public PreviewBoundaryMiddleware(RequestDelegate next, Uri origin, bool loopbackOnly)
    {
        if (origin.Scheme != "http" || origin.Host != "127.0.0.1" || origin.AbsolutePath != "/" || !string.IsNullOrEmpty(origin.Query) || !string.IsNullOrEmpty(origin.Fragment) || !string.IsNullOrEmpty(origin.UserInfo))
        {
            throw new ArgumentException("The private preview requires an http://127.0.0.1:port origin.", nameof(origin));
        }

        _next = next;
        _origin = origin;
        _loopbackOnly = loopbackOnly;
    }

    /// <summary>Rejects foreign hosts/origins before forwarded headers and startup authorization.</summary>
    /// <param name="context">The request context.</param>
    /// <returns>The completion task.</returns>
    public Task InvokeAsync(HttpContext context)
    {
        var request = context.Request;
        var peer = context.Connection.RemoteIpAddress;
        var validPeer = !_loopbackOnly || (peer is not null && IPAddress.IsLoopback(peer));
        var validHost = request.Scheme == _origin.Scheme
            && string.Equals(request.Host.Value, _origin.Authority, StringComparison.OrdinalIgnoreCase);
        var suppliedOrigin = request.Headers.Origin;
        var validOrigin = suppliedOrigin.Count == 0
            || (suppliedOrigin.Count == 1 && string.Equals(suppliedOrigin[0], _origin.GetLeftPart(UriPartial.Authority), StringComparison.Ordinal));
        if (!validPeer || !validHost || !validOrigin)
        {
            context.Response.StatusCode = StatusCodes.Status403Forbidden;
            context.Response.Headers.CacheControl = "no-store";
            return Task.CompletedTask;
        }

        return _next(context);
    }
}
