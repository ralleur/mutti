# Kurt dependency

Kurt's source of truth is the public [ralleur/kurt](https://github.com/ralleur/kurt) repository.
This checkout uses its **mutti** profile. `kurt.lock.json` records the exact
revision and SHA-256 of every managed file. Those files are generated snapshots;
make artwork and reusable animation changes in Kurt, not in this project.

After committing and validating Kurt, run from its checkout:

```sh
npm run export
python3 tools/sync.py --consumer mutti --root /path/to/this-project
python3 tools/sync.py --consumer mutti --root /path/to/this-project --check
```

Sync refuses modified managed files and stale exports. Builds remain offline
and self-contained. Commit the resulting snapshot with the product change;
there is no runtime update or deployment triggered by synchronization.
Project UI, product actions and platform integration remain in this repository.
Existing licensing is retained; see the Kurt notices in the asset directory.
