// SPDX-License-Identifier: GPL-2.0-or-later
using System;
using System.Net;
using System.Threading.Tasks;
using Jellyfin.Server.Mutti;
using Microsoft.AspNetCore.Http;
using Xunit;

namespace Jellyfin.Server.Tests.Mutti;

public class PreviewBoundaryMiddlewareTests
{
    [Theory]
    [InlineData("127.0.0.1:18596", null, "127.0.0.1", true, 204)]
    [InlineData("127.0.0.1:18596", "http://127.0.0.1:18596", "127.0.0.1", true, 204)]
    [InlineData("attacker.example:18596", null, "127.0.0.1", true, 403)]
    [InlineData("127.0.0.1:18596", "https://attacker.example", "127.0.0.1", true, 403)]
    [InlineData("127.0.0.1:18596", "null", "127.0.0.1", true, 403)]
    [InlineData("127.0.0.1:18596", null, "192.168.1.2", true, 403)]
    [InlineData("127.0.0.1:18596", null, "172.20.0.1", false, 204)]
    [InlineData("127.0.0.1:8096", null, "127.0.0.1", true, 403)]
    public async Task InvokeAsync_EnforcesPreviewBoundary(string host, string? origin, string peer, bool loopbackOnly, int expected)
    {
        var context = new DefaultHttpContext();
        context.Request.Scheme = "http";
        context.Request.Host = new HostString(host);
        context.Request.Path = "/Startup/User";
        context.Connection.RemoteIpAddress = IPAddress.Parse(peer);
        if (origin is not null)
        {
            context.Request.Headers.Origin = origin;
        }

        // Spoofing a proxy header must not turn a remote peer into loopback.
        context.Request.Headers["X-Forwarded-For"] = "127.0.0.1";
        var middleware = new PreviewBoundaryMiddleware(
            c =>
            {
                c.Response.StatusCode = 204;
                return Task.CompletedTask;
            },
            new Uri("http://127.0.0.1:18596"),
            loopbackOnly);
        await middleware.InvokeAsync(context);
        Assert.Equal(expected, context.Response.StatusCode);
    }

    [Theory]
    [InlineData("http://0.0.0.0:18596")]
    [InlineData("https://127.0.0.1:18596")]
    [InlineData("http://127.0.0.1:18596/web")]
    [InlineData("http://secret@127.0.0.1:18596")]
    public void Constructor_RejectsUnsafeConfiguration(string origin)
    {
        Assert.Throws<ArgumentException>(() => new PreviewBoundaryMiddleware(_ => Task.CompletedTask, new Uri(origin), true));
    }
}
