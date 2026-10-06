# Mutti

**Deine Medien. Gut zu Hause.**

Mutti ist das Server-Gegenstück zu [kurtz](https://github.com/ralleur/kurtz),
auf Basis von [Jellyfin](https://github.com/jellyfin/jellyfin) v12.1. Die erste
Iteration ist bewusst eine **Mac-App**; das Docker-Paket bleibt als Entwicklerweg
im Repository und folgt später als eigene NAS-Iteration. Beide verwenden denselben
Server und dieselbe [Mutti-Weboberfläche](https://github.com/ralleur/mutti-web).

**Entwicklungsvorschau, kein Release.** Lokale Einrichtung, Marke,
Mac-Prozesssteuerung mit Supervisor, Jellyfin-Import, QR-Gerätekopplung und
direkter verschlüsselter Transport sind als Teststand implementiert und lokal
geprüft, aber nicht abgenommen: kein echter WAN-Test, keine echten Apple-Geräte,
keine Notarisierung. Es gibt keinen eingebauten Relay und keine externe
Kontopflicht; für Netze ohne direkten Weg ist ein Tailscale-Wiki-Artikel geplant.

- [Bauen und lokal ausprobieren](docs/mutti/development.md)
- [Produktplan und Abnahme](docs/mutti/plan.md)
- [Architektur-Review vom 6. Oktober 2026](docs/mutti/review-2026-10-06.md)
- [Umsetzungsstand und Nachweise](docs/mutti/status.md)
- [Jellyfin übernehmen](docs/mutti/import.md) · [Kopplung und Transport](docs/mutti/connect.md)
- [Lizenzprüfung](docs/mutti/licensing.md)
- [Komponenten-Pins](mutti/components.lock.json)
- [Upstream-Updates](docs/mutti/upstream.md)
- [Sicherheit](SECURITY.md) · [Lizenzen](mutti/THIRD-PARTY.md)

Jellyfin-API, Namespaces und Originalhistorie bleiben erhalten. Mutti ist ein
unabhängiges Projekt; die Jellyfin-Mitwirkenden werden in [UPSTREAM.md](UPSTREAM.md)
und den unveränderten Lizenz-/Urheberhinweisen genannt.
