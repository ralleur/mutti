// SPDX-License-Identifier: GPL-2.0-or-later
using System;
using System.Collections.Generic;
using System.Globalization;
using System.IO;
using System.Linq;
using System.Security.Cryptography;
using System.Text;
using System.Text.Json;
using Microsoft.Data.Sqlite;

namespace Mutti.IntroSkipper;

/// <summary>Consistent, read-only-source snapshots of the curated plugin's data.</summary>
internal static class IntroSnapshot
{
    private static readonly string[] DatabaseNames = ["introskipper-v2.db", "introskipper-cache.db", "introskipper.db"];

    internal static void Create(string programData, string output)
    {
        programData = Path.GetFullPath(programData);
        output = Path.GetFullPath(output);
        if (Directory.Exists(output) || File.Exists(output))
        {
            throw new IOException("The snapshot destination must be new.");
        }

        Directory.CreateDirectory(output);
        if (!OperatingSystem.IsWindows())
        {
            File.SetUnixFileMode(output, UnixFileMode.UserRead | UnixFileMode.UserWrite | UnixFileMode.UserExecute);
        }

        var hashes = new SortedDictionary<string, string>(StringComparer.Ordinal);
        foreach (var name in DatabaseNames)
        {
            var source = CheckedPath(programData, "data", "introskipper", name);
            if (!File.Exists(source))
            {
                continue;
            }

            var destination = Path.Combine(output, name);
            using (var input = Open(source, SqliteOpenMode.ReadOnly))
            using (var target = Open(destination, SqliteOpenMode.ReadWriteCreate))
            {
                // SQLite backup includes committed WAL pages. Never copy a live .db file.
                input.BackupDatabase(target);
                using var checkpoint = target.CreateCommand();
                checkpoint.CommandText = "PRAGMA journal_mode=DELETE;";
                checkpoint.ExecuteNonQuery();
            }

            PrivateFile(destination);
            hashes[name] = HashDatabase(destination);
        }

        var configuration = CheckedPath(programData, "plugins", "configurations", "IntroSkipper.xml");
        if (File.Exists(configuration))
        {
            if (new FileInfo(configuration).Length > 1024 * 1024)
            {
                throw new IOException("Intro Skipper configuration is too large.");
            }

            var destination = Path.Combine(output, "IntroSkipper.xml");
            File.Copy(configuration, destination);
            PrivateFile(destination);
            hashes["IntroSkipper.xml"] = Convert.ToHexStringLower(SHA256.HashData(File.ReadAllBytes(destination)));
        }

        File.WriteAllText(Path.Combine(output, "snapshot.json"), JsonSerializer.Serialize(hashes));
        PrivateFile(Path.Combine(output, "snapshot.json"));
    }

    private static string CheckedPath(string root, params string[] parts)
    {
        var path = root;
        foreach (var part in parts)
        {
            path = Path.Combine(path, part);
            if (new FileInfo(path).LinkTarget is not null || new DirectoryInfo(path).LinkTarget is not null)
            {
                throw new IOException("Linked plugin data paths are not accepted.");
            }
        }

        return path;
    }

    private static void PrivateFile(string path)
    {
        if (!OperatingSystem.IsWindows())
        {
            File.SetUnixFileMode(path, UnixFileMode.UserRead | UnixFileMode.UserWrite);
        }
    }

    private static SqliteConnection Open(string path, SqliteOpenMode mode)
    {
        var connection = new SqliteConnection(new SqliteConnectionStringBuilder
        {
            DataSource = path,
            Mode = mode,
            Pooling = false,
            DefaultTimeout = 15,
        }.ToString());
        connection.Open();
        using var command = connection.CreateCommand();
        command.CommandText = "PRAGMA trusted_schema=OFF;";
        command.ExecuteNonQuery();
        return connection;
    }

    // Compare logical rows, not SQLite page layout/checkpoint counters.
    private static string HashDatabase(string path)
    {
        using var connection = Open(path, SqliteOpenMode.ReadOnly);
        using var check = connection.CreateCommand();
        check.CommandText = "PRAGMA quick_check;";
        if (!string.Equals(check.ExecuteScalar() as string, "ok", StringComparison.Ordinal))
        {
            throw new IOException("Intro Skipper database integrity check failed.");
        }

        var tables = new SortedDictionary<string, string>(StringComparer.Ordinal);
        using (var schema = connection.CreateCommand())
        {
            schema.CommandText = "SELECT name, sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name;";
            using var reader = schema.ExecuteReader();
            while (reader.Read())
            {
                tables.Add(reader.GetString(0), reader.GetString(1));
            }
        }

        using var digest = IncrementalHash.CreateHash(HashAlgorithmName.SHA256);
        foreach (var (table, schema) in tables)
        {
            digest.AppendData(Encoding.UTF8.GetBytes(JsonSerializer.Serialize(new[] { table, schema })));
            using var command = connection.CreateCommand();
            command.CommandText = "SELECT * FROM \"" + table.Replace("\"", "\"\"", StringComparison.Ordinal) + "\";";
            using var reader = command.ExecuteReader();
            var rows = new List<string>();
            while (reader.Read())
            {
                var values = new object[reader.FieldCount];
                reader.GetValues(values);
                rows.Add(Convert.ToHexStringLower(SHA256.HashData(JsonSerializer.SerializeToUtf8Bytes(values))));
            }

            rows.Sort(StringComparer.Ordinal);
            digest.AppendData(Encoding.UTF8.GetBytes(rows.Count.ToString(CultureInfo.InvariantCulture) + ":"));
            foreach (var row in rows)
            {
                digest.AppendData(Encoding.UTF8.GetBytes(row));
            }
        }

        return Convert.ToHexStringLower(digest.GetHashAndReset());
    }
}
