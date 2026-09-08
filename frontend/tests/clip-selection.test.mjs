import assert from "node:assert/strict";
import test from "node:test";
import { readFile } from "node:fs/promises";
import ts from "typescript";

async function importTypeScript(relativePath) {
  const sourceURL = new URL(relativePath, import.meta.url);
  const source = await readFile(sourceURL, "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: {
      target: ts.ScriptTarget.ES2020,
      module: ts.ModuleKind.ESNext,
    },
    fileName: sourceURL.pathname,
  });
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`);
}

const {
  fullRoundPlayerSteamID,
  resolveFullRoundPlayerSteamID,
} = await importTypeScript("../src/domains/clip-selection/selection-logic.ts");

test("full-round POV accepts only the lossless SteamID text field", () => {
  assert.equal(
    fullRoundPlayerSteamID({ steam_id: 76561198000000001, steam_id_text: " 76561198000000001 " }),
    "76561198000000001",
  );
  assert.equal(fullRoundPlayerSteamID({ steam_id: 76561198000000001 }), "");
});

test("ordinary roster defaulting preserves the current POV player", () => {
  const players = [
    { steam_id: 76561198000000001, steam_id_text: "76561198000000001" },
    { steam_id: 76561198000000002, steam_id_text: "76561198000000002" },
  ];

  assert.equal(
    resolveFullRoundPlayerSteamID(players, undefined, "76561198000000002"),
    "76561198000000002",
  );
  assert.equal(
    resolveFullRoundPlayerSteamID(players, "76561198000000001", "76561198000000002"),
    "76561198000000001",
  );
  assert.equal(resolveFullRoundPlayerSteamID(players, undefined, "missing"), "76561198000000001");
});
