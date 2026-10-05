// SPDX-License-Identifier: GPL-2.0-or-later
using System;
using System.Collections.Generic;
using System.IO;
using System.Text.Json;
using Microsoft.Data.Sqlite;
using Xunit;

namespace Jellyfin.Server.Tests.Mutti;

public sealed class IntroSnapshotTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), "mutti-intro-test-" + Guid.NewGuid().ToString("N"));

    [Fact]
    public void SnapshotIncludesCommittedWalAndKeepsSourceData()
    {
        var data = Path.Combine(_root, "source", "data", "introskipper");
        Directory.CreateDirectory(data);
        var path = Path.Combine(data, "introskipper-v2.db");
        using var writer = new SqliteConnection(new SqliteConnectionStringBuilder { DataSource = path, Pooling = false }.ToString());
        writer.Open();
        using var command = writer.CreateCommand();
        command.CommandText = "PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0; CREATE TABLE Segments(Id TEXT, StartTicks INTEGER, Fingerprint BLOB); INSERT INTO Segments VALUES('intro', 20, X'00112233');";
        command.ExecuteNonQuery();
        Assert.True(File.Exists(path + "-wal"));
        var before = File.ReadAllBytes(path);
        var config = Path.Combine(_root, "source", "plugins", "configurations");
        Directory.CreateDirectory(config);
        File.WriteAllText(Path.Combine(config, "IntroSkipper.xml"), "<PluginConfiguration><SkipFirstEpisode>true</SkipFirstEpisode></PluginConfiguration>");
        var first = Path.Combine(_root, "first");
        global::Mutti.IntroSkipper.IntroSnapshot.Create(Path.Combine(_root, "source"), first);
        Assert.Equal(before, File.ReadAllBytes(path));
        Assert.Empty(Directory.GetFiles(first, "*-wal"));
        Assert.Empty(Directory.GetFiles(first, "*-shm"));
        using var snapshot = new SqliteConnection(new SqliteConnectionStringBuilder { DataSource = Path.Combine(first, "introskipper-v2.db"), Mode = SqliteOpenMode.ReadOnly, Pooling = false }.ToString());
        snapshot.Open();
        using var read = snapshot.CreateCommand();
        read.CommandText = "SELECT StartTicks FROM Segments WHERE Id='intro';";
        Assert.Equal(20L, read.ExecuteScalar());
        var second = Path.Combine(_root, "second");
        global::Mutti.IntroSkipper.IntroSnapshot.Create(Path.Combine(_root, "source"), second);
        Assert.Equal(File.ReadAllText(Path.Combine(first, "snapshot.json")), File.ReadAllText(Path.Combine(second, "snapshot.json")));
        command.CommandText = "UPDATE Segments SET StartTicks=30 WHERE Id='intro';";
        command.ExecuteNonQuery();
        var changed = Path.Combine(_root, "changed");
        global::Mutti.IntroSkipper.IntroSnapshot.Create(Path.Combine(_root, "source"), changed);
        var a = JsonSerializer.Deserialize<Dictionary<string, string>>(File.ReadAllText(Path.Combine(first, "snapshot.json")))!;
        var b = JsonSerializer.Deserialize<Dictionary<string, string>>(File.ReadAllText(Path.Combine(changed, "snapshot.json")))!;
        Assert.NotEqual(a["introskipper-v2.db"], b["introskipper-v2.db"]);
        Assert.Equal(a["IntroSkipper.xml"], b["IntroSkipper.xml"]);
    }

    [Fact]
    public void SnapshotRejectsSymlinksAndExistingDestination()
    {
        Directory.CreateDirectory(Path.Combine(_root, "source", "data"));
        Directory.CreateDirectory(Path.Combine(_root, "outside"));
        Directory.CreateSymbolicLink(Path.Combine(_root, "source", "data", "introskipper"), Path.Combine(_root, "outside"));
        Assert.Throws<IOException>(() => global::Mutti.IntroSkipper.IntroSnapshot.Create(Path.Combine(_root, "source"), Path.Combine(_root, "snapshot")));
        Assert.Throws<IOException>(() => global::Mutti.IntroSkipper.IntroSnapshot.Create(Path.Combine(_root, "source"), Path.Combine(_root, "outside")));
    }

    public void Dispose()
    {
        if (Directory.Exists(_root))
        {
            Directory.Delete(_root, recursive: true);
        }
    }
}
