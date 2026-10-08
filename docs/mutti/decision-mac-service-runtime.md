# Entscheidungsvorlage: Laufzeit für Linux-Dienste auf dem Mac (offene Entscheidung 1)

> **Entschieden am 07.10.2026:** hybrid mit eingebetteter Container-Runtime
> (Apples Containerization-Framework), nativ nur für Apple-Hardware; Umsetzung
> als M8. Maßgeblich ist [plan.md, Abschnitt 16](plan.md#16-weitere-dienste-laufzeitmodell-und-upstream-treue).
> Diese Vorlage bleibt als Entscheidungsgrundlage erhalten.

Stand 06.10.2026, für Day 1 „Mac only, Apple Silicon, MLX“. Betrifft P2
„Verwalteter Betrieb“: Immich (Fotos) und Paperless-ngx (Dokumente) sind
Linux-Containerdienste mit Datenbank/Queue (Postgres, Valkey/Redis). Heute bindet
Mutti **vorhandene** Instanzen an (Profil-Konten, keine Installation durch Mutti);
siehe [modules.md](modules.md). Diese Vorlage entscheidet, ob und womit Mutti sie
selbst installiert, aktualisiert und sichert.

## Optionen

| Option | Lizenz / Weitergabe | Betrieb auf dem Nutzer-Mac | Bewertung |
| --- | --- | --- | --- |
| **A. Apple `container`** (Containerization, Swift) | Apache-2.0; als Framework einbettbar | Nur Apple Silicon und **macOS 26**; je Container eine leichte VM; v1.4.1 (Sept. 2026); kein Docker-Compose, Orchestrierung müsste Mutti selbst übernehmen | Passt zu „Mac only“ und braucht keine Fremdinstallation. Für fünf zusammenhängende Dienste ist das noch Neuland (Netz zwischen Containern, Ressourcen pro VM). |
| **B. Lima/Colima** (eine Linux-VM mit containerd) | Lima Apache-2.0, Colima MIT; bündelbar | Eine VM für alle Dienste, Compose über nerdctl, ausgereift; braucht eigenes VM-Image im Paket und dessen Updates | Technisch der sicherste Weg zu einem verwalteten Stack; mehr Paketgröße und Wartung. |
| **C. Docker Desktop voraussetzen** | Kostenlos für privat/kleine Firmen, nicht bündelbar | Nutzer installiert und pflegt Docker selbst | Widerspricht „keine Terminal-/Docker-Kenntnisse nötig“ (Risiko in `decisions.md`). |
| **D. OrbStack voraussetzen** | Privat frei, kommerziell lizenzpflichtig, nicht bündelbar | Wie C | Wie C, zusätzlich Lizenzfrage bei gewerblicher Nutzung. |
| **E. Day 1 ohne verwaltete Dienste** | – | Mutti bindet vorhandenes Immich/Paperless an (bereits umgesetzt und geprüft) | Kein zusätzliches Day-1-Risiko; verwalteter Betrieb wird eigenes Inkrement. |

## Empfehlung

**Day 1: E** (Anbindung vorhandener Dienste wie heute). Verwalteter Betrieb als
nächstes Inkrement mit einem kurzen Machbarkeitsnachweis **A gegen B** auf dem
Ziel-Mac: kompletter Immich- und Paperless-Stack, isolierte Netze,
schreibgeschützte Medien-Mounts, Sicherung/Restore, Update mit Rücksprung,
Speicher- und Startzeit. Danach Festlegung. C und D scheiden für ein
„ohne Docker-Wissen“-Produkt aus.

Folgefrage zu A: Mindestversion macOS 26 für die Server-App. Day 1 qualifiziert
ohnehin nur exakt gemessene Apple-Silicon-Macs.

## Quellen

- [apple/container (GitHub, Apache-2.0)](https://github.com/apple/container),
  [macOS 26 native Linux-Container (AppleInsider)](https://appleinsider.com/articles/25/06/09/sorry-docker-macos-26-adds-native-support-for-linux-containers),
  [Compose-Lücke und Migration (wavect)](https://wavect.io/blog/apple-container-vs-docker-compose-migration/)
- [Docker Desktop license agreement](https://docs.docker.com/subscription/desktop-license/),
  [OrbStack licensing](https://docs.orbstack.dev/licensing),
  [Lima (Apache-2.0)](https://lima-vm.io/docs/installation/_print/), Colima (MIT)
