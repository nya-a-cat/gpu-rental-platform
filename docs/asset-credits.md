# Visual asset credits

The interface is built from React components and CSS/SVG controls. No generated mockup is used as a page image.

## Public-domain archive images

- **INSTRUMENT PANELS IN CONTROL ROOM** — photograph by Martin Brown, NASA John H. Glenn Research Center, held by NARA. Public domain in the United States under `PD-USGov-NASA`. The application loads a Wikimedia Commons thumbnail and links to the [file page](https://commons.wikimedia.org/wiki/File:INSTRUMENT_PANELS_IN_CONTROL_ROOM_-_NARA_-_17447770.jpg). NASA names and marks do not imply endorsement.
- **Lunokhod control panel 1** — photograph by Regnard, released into the public domain. The application loads a Wikimedia Commons thumbnail and links to the [file page](https://commons.wikimedia.org/wiki/File:Lunokhod_control_panel_1.jpg). The image is an archival design reference and does not represent platform hardware.

Both remote images are optional visual layers. Core navigation and workflows remain usable if Wikimedia Commons is unavailable.

## Original generated assets

- `apps/web/src/assets/generated/gpu-module-cutaway.webp` — original fictional compute-module illustration generated for this project. It does not depict a vendor product.
- `apps/web/src/assets/generated/mechanical-status-plate.webp` — original decorative mechanical plate generated for this project. Its abstract dials are not GPU telemetry.
- `apps/web/src/assets/generated/inspection-calibration-plate.webp` — original enamel calibration strip generated as a small material component. It contains no controls or telemetry and is composited with functional HTML controls.

The generated assets are distributed with this repository under the project license. They are separate components combined with the coded interface, not full-page screenshots.

## Typography

The interface loads [Barlow Condensed](https://fonts.google.com/specimen/Barlow+Condensed), [IBM Plex Mono](https://fonts.google.com/specimen/IBM+Plex+Mono) and [Noto Sans SC](https://fonts.google.com/noto/specimen/Noto+Sans+SC) from Google Fonts with `font-display: swap`. These fonts are distributed under the SIL Open Font License. System fallbacks preserve the workflows when the font service is unavailable.
