# CityEvents Frontend

Phase 8 uses a dependency-free browser app because this workspace has Node available but no npm package manager.

Run locally:

```powershell
.\scripts\serve-frontend.ps1
```

Then open:

```text
http://127.0.0.1:18088
```

The app calls local services directly:

- auth: `http://127.0.0.1:8081`
- event registration: `http://127.0.0.1:8082`
- feed: `http://127.0.0.1:8083`
- media: `http://127.0.0.1:8085`

The gateway/BFF remains a later hardening step.
