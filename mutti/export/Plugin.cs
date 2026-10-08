// SPDX-License-Identifier: GPL-2.0-or-later
using MediaBrowser.Common.Plugins;
namespace Mutti.Export;
public sealed class Plugin : BasePlugin
{
    public Plugin()
    {
        var assembly = typeof(Plugin).Assembly;
        SetAttributes(assembly.Location, Path.GetDirectoryName(assembly.Location)!, assembly.GetName().Version!);
    }
    public override string Name => "Mutti Export";
    public override string Description => "Zeitlich begrenzter, administratorbestätigter Umzug nach Mutti. Keine Registrierung, kein Relay.";
    public override Guid Id => Guid.Parse("e4f00df3-097a-4b0f-bd9b-822d069d36c0");
}
