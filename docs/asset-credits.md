# Visual asset credits

The interface is built from React components and CSS/SVG controls. No generated mockup is used as a page image.

## Public-domain archive images

- **INSTRUMENT PANELS IN CONTROL ROOM** — photograph by Martin Brown, NASA John H. Glenn Research Center, held by NARA. Public domain in the United States under `PD-USGov-NASA`. A local 1280-pixel Wikimedia Commons derivative is stored at `apps/web/src/assets/archive/nasa-control-room-1976.jpg` and used on the market and identity pages; both views link to the [file page](https://commons.wikimedia.org/wiki/File:INSTRUMENT_PANELS_IN_CONTROL_ROOM_-_NARA_-_17447770.jpg). NASA names and marks do not imply endorsement.
- **Lunokhod control panel 1** — photograph by Regnard, released into the public domain. The application loads a Wikimedia Commons thumbnail and links to the [file page](https://commons.wikimedia.org/wiki/File:Lunokhod_control_panel_1.jpg). The image is an archival design reference and does not represent platform hardware.

The NASA photograph is bundled with the production build, so its atmospheric layer does not depend on Wikimedia Commons at runtime. Remote archive images remain optional visual layers; core navigation and workflows remain usable when they are unavailable.

## Original generated assets

- `apps/web/src/assets/generated/gpu-module-cutaway.webp` — original fictional compute-module illustration generated for this project. It does not depict a vendor product.
- `apps/web/src/assets/generated/mechanical-status-plate.webp` — original decorative mechanical plate generated for this project. Its abstract dials are not GPU telemetry.
- `apps/web/src/assets/generated/inspection-calibration-plate.webp` — original enamel calibration strip generated as a small material component. It contains no controls or telemetry and is composited with functional HTML controls.
- `apps/web/src/assets/generated/silver-rack-wall.webp` — original silver industrial rack-wall texture generated as a wide environmental layer for the inventory workbench. It contains no product UI or functional controls.
- `apps/web/src/assets/generated/silver-service-duct.webp` — original service-duct texture generated as a shallow status-bus layer. It contains no text, logos, screens or functional controls.

The generated assets are distributed with this repository under the project license. They are separate components combined with the coded interface, not full-page screenshots.

## Typography

The interface loads [Barlow Condensed](https://fonts.google.com/specimen/Barlow+Condensed), [IBM Plex Mono](https://fonts.google.com/specimen/IBM+Plex+Mono) and [Noto Sans SC](https://fonts.google.com/noto/specimen/Noto+Sans+SC) from Google Fonts with `font-display: swap`. These fonts are distributed under the SIL Open Font License. System fallbacks preserve the workflows when the font service is unavailable.
