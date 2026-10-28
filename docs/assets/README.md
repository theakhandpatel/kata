# Logo assets

`logo.png` is the source image with its background removed, cropped to the subject and
scaled to 512 pixels on the long edge. `logo-128.png` is a padded 128 pixel square, used as
the navigation logo and the apple touch icon. `favicon.ico` is the same art at 48 pixels.

All three are generated from one supplied image. Nothing here was drawn by hand.

## How they were made

The source is a 2000 by 1116 PNG with a fully opaque, near-white photographic backdrop. It
was processed by a small Go program using only the standard library, because Pillow,
numpy and ImageMagick are not available in this environment.

The pipeline:

1. **Background removal.** A flood fill from every border pixel, not a global colour test.
   The fill starts at the image border and spreads through pixels within a tolerance of 34
   of the median border colour. A flood fill is used so that near-white pixels *inside* the
   subject, such as a highlight or the whites of eyes, are never touched. It cleared 82.2%
   of the image.
2. **De-matte.** The source was photographed against white, so the anti-aliased boundary
   pixels still carried white after the background was keyed out. That is invisible on a
   white page and a visible glow on a dark one. Every partial-alpha pixel therefore takes
   the colour of its nearest fully opaque neighbour while keeping its own alpha, so the
   edge carries artwork colour and composites correctly on any background. Pixels brighter
   than their neighbours fell from 5.0% to 0.0%.
3. **Crop and pad.** The tight bounding box of every pixel with alpha above 8, which gave
   855 by 853 pixels starting at (576, 123), then a 4% transparent margin so no artwork
   touches the canvas edge. Without the margin, 57 visible pixels sat on the border.
4. **Downscale.** Box averaging rather than nearest neighbour, done in premultiplied alpha
   so transparent pixels do not darken the edge toward black.
5. **Quantise.** Colour was reduced to 8 levels per channel, with alpha kept exact. That cut
   the PNG from 341 KB to 103 KB with no visible change at these sizes.
6. **Square pad.** `logo-128.png` is centred on a square canvas with 6% margin so it never
   touches the edge of a navigation bar.

## Verification

Measured, not assumed:

| Property | Result |
|---|---|
| Transparent pixels | 82.2% of the source |
| Corner pixels | fully transparent on all four corners |
| Boundary luminance | edges matted against dark, 0.0% whitish anti-alias pixels |
| White halo | none, verified numerically and by an independent visual review |
| Bright-rim pixels over a dark page | 7, down from 1925 before the de-matte |
| Artwork touching the canvas edge | 0, down from 57 before the pad |
| Opaque mean luminance | 147.6, so it reads on a light page (contrast 107) and a dark one (contrast 134) |
| Size at 128 px | still recognisable |

## Regenerating

The generator is not committed, because it was a one-off. If the source image changes, redo
the five steps above with any image library: flood fill from the borders at tolerance 34,
crop to non-transparent pixels, box-downscale, quantise, pad.

If you replace the art, update the width in the README's `<img>` tag and re-run
`miyagi site` so the copied files under the site's public directory are refreshed.
