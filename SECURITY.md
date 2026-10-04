# Security

Mutti is currently a local development preview. No production security or
reachability guarantee is made for unfinished pairing / remote features.
Keep it bound to loopback as described in the server development guide.

Report vulnerabilities privately using [GitHub security reporting](https://github.com/ralleur/mutti/security/advisories/new).
Do not put credentials, pairing material, user databases or media names in public
issues. Upstream Jellyfin vulnerabilities should also follow the upstream
[security policy](https://github.com/jellyfin/jellyfin/security/policy).

Release criteria include device-bound trust, short-lived single-use pairing,
least-privilege playback, immediate revocation and household isolation. These
criteria are not satisfied merely by turning on Jellyfin Quick Connect.
