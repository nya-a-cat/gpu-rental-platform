# GitHub Pages demo mode

The GitHub Pages site is an interactive product demonstration. It is intentionally separate from the Docker profile and always displays that it is not connected to a real backend.

## What it demonstrates

- GPU model filtering and resource detail views
- Interactive console controls for availability, price ceiling and inventory ordering
- Operator login, duration presets, reservation confirmation and one-step return
- Administrator overview, resource registration, listing state changes and order cancellation
- Role-based navigation and page access behavior
- Desktop and mobile interface states

The demo adapter uses deterministic sample inventory and browser-local state. Operator and dispatcher shortcuts exercise the same page routes and state transitions as the API adapter. Resetting the demo restores the original dataset, making the public walkthrough repeatable.

Seeded and administrator-created demo resources display their complete `GPU-<sequence>` record label. Real API object identifiers remain shortened to their final six characters in market cards.

The market console is not decorative. Its control-bus switch enables or disables the quick-control bank, the availability lock updates the inventory filter, and the three rotary controls cycle availability, price ceiling and sort order. Their values stay synchronized with the standard form controls and inventory results.

The surrounding archive photograph, silver rack wall and service duct are bundled environmental layers. The status bridge mirrors the active control-bus state, availability, price ceiling, sorting mode and current inventory count; disconnecting the control bus visibly powers down the console without hiding the standard filters.

The archive image is removed from grid sizing so the desktop hero remains within its 680–820 pixel design band. On narrow screens the photograph, gauges and status bridge use a compact layout while preserving all six interactive console controls.

## What it does not claim

Demo mode does not allocate physical GPUs, start containers, accept payments, open SSH or notebook sessions, or report live utilization, temperature, IP addresses or host status. Any resource and order changes exist only in the current browser profile.

No MongoDB, Redis or NestJS service runs on GitHub Pages. The static build must not send API requests. Backend behavior, session revocation and concurrent reservation safety must be verified through the real API profile and its integration tests.

## Switching profiles

The build-time `VITE_RUNTIME_MODE` value selects the data adapter:

- `demo` uses the transparent browser-only adapter for GitHub Pages.
- `api` uses same-origin `/api` requests for the Docker deployment.

The mode is fixed when the web assets are built. It is not a user-facing switch, which prevents a public static deployment from appearing to connect to infrastructure that is not present.
