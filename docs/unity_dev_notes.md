# Unity WebGL Build Notes for Developers

Notes and requests for the Unity development team regarding WebGL builds for the MHS content delivery system. These changes are not urgent — the current builds work as-is — but should be incorporated in future builds to improve performance and simplify deployment.

## 1. Enable Brotli Compression

**Current:** Builds output `.unityweb` files (no compression or gzip).

**Requested:** Enable Brotli compression in Unity's WebGL build settings so files are output with `.br` extensions (e.g., `unit1.data.br`, `unit1.wasm.br`, `unit1.framework.js.br`).

**Why:** Brotli-compressed builds are significantly smaller (typically 20-30% smaller than gzip). Since we're pre-downloading game builds over the network and caching them on student Chromebooks, smaller files mean faster downloads, less bandwidth usage in classrooms, and less local storage consumed. The browser decompresses transparently — there's no runtime cost.

**How in Unity:**
- Open **Edit > Project Settings > Player > WebGL > Publishing Settings**
- Set **Compression Format** to **Brotli**
- This changes the output file extensions from `.unityweb` to `.br`

**Note:** Our CDN (CloudFront) and web server are already configured to handle `.br` files correctly. Once the builds switch to Brotli, we'll update the content manifest and launcher accordingly.

## 2. Consistent Build Naming Convention

**Current:** Build files are named after the Unity project/scene (e.g., `Unit1WebResizeTest.loader.js`).

**Requested:** Use a consistent, short naming convention for each unit's build output:
- Unit 1: `unit1.loader.js`, `unit1.data.br`, `unit1.framework.js.br`, `unit1.wasm.br`
- Unit 2: `unit2.loader.js`, `unit2.data.br`, etc.
- Unit 3-5: same pattern

**Why:** The content delivery system constructs file URLs programmatically based on the unit ID. Consistent naming eliminates manual renaming and reduces the chance of errors when uploading new builds. It also makes the CDN file structure clean and predictable.

**How in Unity:**
- In **Build Settings**, set the build output name to `unit1`, `unit2`, etc. for each unit before building
- Unity will generate files named `unit1.loader.js`, `unit1.data.br`, etc.

## 3. Addressables Note

The current builds include Unity Addressables (StreamingAssets/aa/ folder) for the Localization package. This is fine — the content delivery system handles these files alongside the main build files. No changes needed here.

If in the future the Localization package is no longer needed or is replaced with a simpler solution, removing Addressables from the WebGL build would simplify the output (fewer files, fewer runtime HTTP requests), but this is low priority.

## 4. Build Output Structure

For reference, here is how we expect the build output to be organized. When handing off a build for a unit, please provide:

```
Build/
  unit1.loader.js              (bootstrap loader - not compressed)
  unit1.data.br                (game data - brotli compressed)
  unit1.framework.js.br        (Unity framework - brotli compressed)
  unit1.wasm.br                (WebAssembly - brotli compressed)
StreamingAssets/
  UnityServicesProjectConfiguration.json
  aa/
    catalog.bin
    catalog.hash
    settings.json
    WebGL/
      (addressable bundle files)
    AddressablesLink/
      link.xml
```

We do **not** need the `index.html` from the build output — we have our own game launcher page that handles loading, progress display, and integration with StrataHub.

## 5. Version Tagging

When delivering a new build, please include a version identifier (e.g., `1.0.0`, `1.1.0`). We use version numbers in the CDN path structure to manage cache invalidation without requiring CloudFront invalidations:

```
cdn.adroit.games/mhs/unit1/v1.0.0/unit1.data.br
cdn.adroit.games/mhs/unit1/v1.1.0/unit1.data.br   (new version = new path)
```

The version can be communicated via the build handoff (email, ticket, etc.) — it doesn't need to be embedded in the filenames.

## 6. Identity Bridge

The JavaScript identity bridge (`.jslib` file in Assets/Plugins/) that communicates player identity from StrataHub to Unity is baked into the `framework.js` during the WebGL build. Our game launcher page serves from the same StrataHub origin, so the session cookie is available and the bridge should work without any additional JavaScript on the hosting page.

If the bridge implementation needs to change or if there are issues with identity detection, please coordinate so we can make any necessary adjustments to the hosting page.
