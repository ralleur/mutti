// Rasterize the canonical, outlined SVG artwork. No fonts or remote assets needed.
// node render-brand.cjs /path/to/@resvg/resvg-js [/path/to/mutti-web]
const fs = require('node:fs');
const path = require('node:path');
const {Resvg} = require(process.argv[2] || '@resvg/resvg-js');
const root = path.resolve(__dirname, '../..');
const design = path.join(root, 'mutti/design');
const web = path.resolve(process.argv[3] || path.join(root, '../mutti-web'));
const source = fs.readFileSync(path.join(design, 'assets/symbol.svg'));
const raster = (svg, size) => new Resvg(svg, {fitTo: {mode: 'width', value: size}, font: {loadSystemFonts: false}}).render().asPng();
for (const size of [72, 114, 144, 180, 512]) {
    fs.writeFileSync(path.join(web, 'src/assets/mutti', `touchicon${size === 180 ? '' : size}.png`), raster(source, size));
}
const iconset = path.join(root, 'build/Mutti.iconset');
fs.mkdirSync(iconset, {recursive: true});
for (const size of [16, 32, 128, 256, 512]) for (const scale of [1, 2]) {
    fs.writeFileSync(path.join(iconset, `icon_${size}x${size}${scale === 2 ? '@2x' : ''}.png`), raster(source, size * scale));
}
fs.writeFileSync(path.join(design, 'assets/wordmark-light.png'), raster(fs.readFileSync(path.join(design, 'assets/wordmark-light.svg')), 282));
console.log('Rendered web icons, native wordmark and macOS iconset.');
