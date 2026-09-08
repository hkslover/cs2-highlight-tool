import { mkdir, copyFile } from "node:fs/promises";
import { dirname, resolve } from "node:path";

// The emitted edit domain keeps its project-root-relative
// `../../../wailsjs/runtime/runtime.js` import. From the generated
// `domains/edit/editDomain.js` that resolves to the cache root, so mirror the
// generated runtime there for Node's ESM resolver.
const target = resolve("node_modules/.cache/wailsjs/runtime/runtime.js");
await mkdir(dirname(target), { recursive: true });
await copyFile(resolve("wailsjs/runtime/runtime.js"), target);
