# Deployment requirements (production) — Backend API

This document collects requirements that must be met for a **production** deployment of the `ebo-planner-backend` service.

## CORS

- **Requirement**: Match the CORS policy implied by the deployment proxy configuration (see `deploy/Caddyfile`), but be **more restrictive** in production (explicit allow-list of origins; avoid wildcards).

