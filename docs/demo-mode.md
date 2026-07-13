# GitHub Pages demo mode

The GitHub Pages site is an interactive product demonstration. It is intentionally separate from the Docker profile and always displays that it is not connected to a real backend.

## What it demonstrates

- GPU model filtering and resource detail views
- Interactive console controls for availability, price ceiling and inventory ordering
- Operator login, duration presets, reservation confirmation and one-step return
- Administrator overview, resource registration, listing state changes and order cancellation
- Role-based navigation and page access behavior
- Desktop and mobile interface states

The demo adapter uses deterministic sample inventory and browser-local state. Operator and dispatcher shortcuts exercise the same page routes and state transitions as the API adapter. Resetting the demo restores the original dataset, making the public walkthrough repeatable. The interactive-console preview uses the independent `gpu-rental-demo-state-v2` storage namespace, so its orders and resource changes cannot alter the frozen classic preview.

The Chinese or English interface preference persists in the browser. Reloading the site restores both the visible copy and the document language metadata.

New operator usernames accept ASCII letters, digits, underscores and hyphens. The browser validates this rule before registration is submitted.

Seeded and administrator-created demo resources display their complete `GPU-<sequence>` record label. Real API object identifiers remain shortened to their final six characters in market cards.

The market console is not decorative. Its control-bus switch enables or disables the quick-control bank, the availability lock updates the inventory filter, and the three rotary controls cycle availability, price ceiling and sort order. Their values stay synchronized with the standard form controls, control-offset meter and inventory results. Rotary controls also support both directions through the arrow keys.

The live inventory rack displays up to three resources from the current result set. Every row opens the real resource detail route, while the matched count, status lamps and prices update from the same filtered inventory used by the full market grid.

The surrounding archive photograph, silver rack wall and service duct are bundled environmental layers. The status bridge mirrors the active control-bus state, availability, price ceiling, sorting mode and current inventory count; disconnecting the control bus visibly powers down the console without hiding the standard filters.

The interactive-console preview keeps the desktop hero within a 540–590 pixel design band, exposing the full inventory heading at the first viewport boundary while keeping real resource links inside the hero. On narrow screens the navigation returns to document flow, the photograph, gauges and status bridge use a compact layout, and all six console controls retain a minimum 44-pixel touch target.

## What it does not claim

Demo mode does not allocate physical GPUs, start containers, accept payments, open SSH or notebook sessions, or report live utilization, temperature, IP addresses or host status. Any resource and order changes exist only in the current browser profile.

No MongoDB, Redis or NestJS service runs on GitHub Pages. The static build must not send API requests. Backend behavior, session revocation and concurrent reservation safety must be verified through the real API profile and its integration tests.

## Switching profiles

The build-time `VITE_RUNTIME_MODE` value selects the data adapter:

- `demo` uses the transparent browser-only adapter for GitHub Pages.
- `api` uses same-origin `/api` requests for the Docker deployment.

The mode is fixed when the web assets are built. It is not a user-facing switch, which prevents a public static deployment from appearing to connect to infrastructure that is not present.
