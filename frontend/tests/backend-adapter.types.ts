import { backend, callBackend } from "../src/shared/backend";
import type {
  BackendApi,
  BackendMethod,
  BackendResult,
} from "../src/shared/backend";
import type { ClipSettings, GeneratePluginJSONRequest } from "../src/shared/types";
import type { WanmeiMatchListResult } from "../src/shared/types/import";

type GeneratedAppModule = typeof import("../wailsjs/go/app/App");
type GeneratedMethod = keyof GeneratedAppModule;

type AssertNever<T extends never> = T;
type _EveryGeneratedMethodIsMapped = AssertNever<
  Exclude<GeneratedMethod, BackendMethod>
>;
type _EveryMappedMethodIsGenerated = AssertNever<
  Exclude<BackendMethod, GeneratedMethod>
>;

type Equal<A, B> =
  (<T>() => T extends A ? 1 : 2) extends
  (<T>() => T extends B ? 1 : 2) ? true : false;
type Expect<T extends true> = T;
type _ApiKeySetMatchesGeneratedBinding = Expect<
  Equal<keyof BackendApi, GeneratedMethod>
>;

type GeneratedArgs<M extends GeneratedMethod> = GeneratedAppModule[M] extends (
  ...args: infer Args
) => unknown
  ? Args
  : never;
type _MethodAritiesMatchGeneratedBinding = Expect<
  {
    [M in BackendMethod]: Equal<BackendApi[M] extends (...args: infer Args) => unknown ? Args["length"] : never, GeneratedArgs<M>["length"]>;
  }[BackendMethod] extends true
    ? true
    : false
>;
const demoRequest = {} as GeneratePluginJSONRequest;
const clipSettingsResult: Promise<ClipSettings> = callBackend("GetClipSettings");
const matchListResult: Promise<WanmeiMatchListResult> = callBackend(
  "ListWanmeiRecentMatches",
  1,
);
const importedPathsResult: Promise<string[]> = backend.ImportWanmeiMatch("match");
const typedResult: Promise<ClipSettings> = backend.GetClipSettings();
const generatedResult: Promise<BackendResult<"GeneratePluginJSON">> =
  callBackend("GeneratePluginJSON", demoRequest);

void clipSettingsResult;
void matchListResult;
void importedPathsResult;
void typedResult;
void generatedResult;

// @ts-expect-error Method names must come from the App contract.
callBackend("NotAnAppMethod");
// @ts-expect-error AckChangelog requires its version argument.
callBackend("AckChangelog");
// @ts-expect-error Method arguments are checked against their mapped tuple.
callBackend("ListWanmeiRecentMatches", "1");
// @ts-expect-error Return values cannot be replaced by a caller assertion.
const wrongReturnType: Promise<string> = callBackend("GetClipSettings");

void wrongReturnType;
