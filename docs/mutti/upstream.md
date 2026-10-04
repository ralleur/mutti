# Upstream-Integration

`upstream` zeigt auf jellyfin/jellyfin bzw. jellyfin/jellyfin-web; `origin` auf
ralleur/mutti bzw. ralleur/mutti-web. Die Mutti-Produktlinie beginnt bei den
zusammenpassenden Tags v12.1, nicht beim ungeprüften Entwicklungsstand.

Für jedes Update einen `codex/upstream-<version>`-Branch erstellen, beide stabilen
Tags prüfen, Upstream-Historie integrieren und Mutti-Anpassungen nachziehen.
`components.lock.json` aktualisieren; kein automatisches Verteilen von `master`.
Ein Sicherheitsfix kann vorab als dokumentierter Backport übernommen werden.

Pflicht vor Freigabe: Server/Web-Build, Zugriffstests, Einrichtung, Wiedergabe,
Kopplung/Widerruf sobald implementiert, Mac und Docker, Upgrade auf einer Datenkopie
und Restore des alten Datenstands. Ein Datenbank-Downgrade ist kein Rückweg.
Upstream-Konflikte nicht durch Löschen von Mutti-Sicherheitsgrenzen lösen.

Release-Manifest, Image-Digests, Signaturen, passende Quellen und Lizenzinventar
gehören zusammen. Dieser Prozess legt keinen neuen geplanten Hintergrundjob an
und verändert keine bestehende kurtz-Automation.
