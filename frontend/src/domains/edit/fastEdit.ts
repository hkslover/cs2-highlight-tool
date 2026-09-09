import type { ProduceHistoryItem } from "@/shared/types";
import type { EditSequenceItem } from "./types";

export const OPPONENT_FAST_EDIT_MAX_PRE_SECONDS = 1;
export const OPPONENT_FAST_EDIT_INTERMEDIATE_POST_SECONDS = 0.1;
export const OPPONENT_FAST_EDIT_GROUP_END_POST_SECONDS = 0.3;
export const OPPONENT_FAST_EDIT_ALLOWED_OVERLAP_SECONDS = 0.1;
export const OPPONENT_FAST_EDIT_MIN_NEXT_LEAD_SECONDS = 0.15;
export const OPPONENT_FAST_EDIT_MIN_CLIP_SECONDS = 0.05;
// The backend's lowest supported edit FPS is 24 and its filter graph requires
// at least two frames for a clip that takes the filtered path.
export const OPPONENT_FAST_EDIT_SAFE_MIN_SECONDS = 2 / 24;

export interface EditTrimRange {
  start_seconds: number;
  end_seconds: number;
}

export interface OpponentFastEditPlan {
  ranges: Array<EditTrimRange | undefined>;
  /** A true entry means the gap after that sequence item must stay a hard cut. */
  hardCutAfter: boolean[];
}

interface FastEditEvent {
  offsetSeconds: number;
}

interface FastEditClip {
  index: number;
  duration: number;
  events: FastEditEvent[];
  groupKey: string;
  recordBaseSeconds: number;
}

/**
 * Plans optional trims for contiguous victim-view clips. The source sequence
 * is never reordered or mutated; a missing/old timing marker simply leaves
 * that clip as a full-length clip.
 */
export function buildOpponentFastEditPlan(
  sequenceItems: readonly EditSequenceItem[],
): OpponentFastEditPlan {
  const ranges: Array<EditTrimRange | undefined> = new Array<EditTrimRange | undefined>(
    sequenceItems.length,
  ).fill(undefined);
  const hardCutAfter = new Array<boolean>(Math.max(0, sequenceItems.length - 1)).fill(false);
  const candidates = sequenceItems
    .map((item, index) => toFastEditClip(item, index))
    .filter((item): item is FastEditClip => item !== undefined);

  let groupStart = 0;
  while (groupStart < candidates.length) {
    let groupEnd = groupStart + 1;
    while (
      groupEnd < candidates.length &&
      candidates[groupEnd].index === candidates[groupEnd - 1].index + 1 &&
      candidates[groupEnd].groupKey === candidates[groupEnd - 1].groupKey
    ) {
      groupEnd++;
    }

    for (let offset = groupStart; offset < groupEnd; offset++) {
      const clip = candidates[offset];
      const firstEvent = clip.events[0].offsetSeconds;
      const lastEvent = clip.events[clip.events.length - 1].offsetSeconds;
      const isGroupEnd = offset === groupEnd - 1;
      let start = Math.max(0, firstEvent - OPPONENT_FAST_EDIT_MAX_PRE_SECONDS);
      const post = isGroupEnd
        ? OPPONENT_FAST_EDIT_GROUP_END_POST_SECONDS
        : OPPONENT_FAST_EDIT_INTERMEDIATE_POST_SECONDS;
      const end = Math.min(clip.duration, lastEvent + post);

      if (offset > groupStart) {
        const previous = candidates[offset - 1];
        const previousRange = ranges[previous.index];
        const previousEndAbsolute = absoluteBoundary(previous, previousRange);
        const currentBaseAbsolute = clip.recordBaseSeconds;
        const currentEventAbsolute = currentBaseAbsolute + firstEvent;
        const previousEventAbsolute =
          previous.recordBaseSeconds + previous.events[0].offsetSeconds;
        // A manually reversed sequence is still kept in user order, but the
        // later source event must not push the earlier take's in-point past
        // its own death marker. Each reversed take stays independently tight.
        if (previousEndAbsolute !== undefined && currentEventAbsolute >= previousEventAbsolute) {
          const rawStartAbsolute = currentBaseAbsolute + start;
          const minimumStartAbsolute = previousEndAbsolute - OPPONENT_FAST_EDIT_ALLOWED_OVERLAP_SECONDS;
          if (rawStartAbsolute < minimumStartAbsolute) {
            const latestStartWithLead =
              currentEventAbsolute - OPPONENT_FAST_EDIT_MIN_NEXT_LEAD_SECONDS;
            // Prefer the overlap cap when it leaves the next death's short
            // lead-in intact. If the two constraints conflict, preserve the
            // lead-in and allow the necessary short overlap instead.
            const desiredStartAbsolute = Math.min(
              Math.max(rawStartAbsolute, minimumStartAbsolute),
              latestStartWithLead,
            );
            start = desiredStartAbsolute - currentBaseAbsolute;
            const minimumLength = Math.min(OPPONENT_FAST_EDIT_MIN_CLIP_SECONDS, end / 2);
            start = Math.min(start, end - minimumLength);
          }
        }
      }

      start = clamp(start, 0, clip.duration);
      const boundedEnd = clamp(end, 0, clip.duration);
      const minimumLength = Math.min(OPPONENT_FAST_EDIT_MIN_CLIP_SECONDS, boundedEnd / 2);
      if (boundedEnd - start < minimumLength) {
        start = Math.max(0, boundedEnd - minimumLength);
      }
      if (boundedEnd > start) {
        const roundedStart = roundSeconds(start, clip.duration);
        const roundedEnd = roundSeconds(boundedEnd, clip.duration);
        const roundedLength = roundedEnd - roundedStart;
        if (
          roundedStart > 0 || roundedEnd < clip.duration
        ) {
          if (roundedLength >= OPPONENT_FAST_EDIT_SAFE_MIN_SECONDS) {
            ranges[clip.index] = {
              start_seconds: roundedStart,
              end_seconds: roundedEnd,
            };
          }
        }
      }

      if (offset > groupStart && ranges[clip.index] && ranges[candidates[offset - 1].index]) {
        hardCutAfter[clip.index - 1] = true;
      }
    }

    groupStart = groupEnd;
  }
  return { ranges, hardCutAfter };
}

export function buildOpponentFastEditRanges(
  sequenceItems: readonly EditSequenceItem[],
): Array<EditTrimRange | undefined> {
  return buildOpponentFastEditPlan(sequenceItems).ranges;
}

export function buildOpponentFastEditHardCuts(
  sequenceItems: readonly EditSequenceItem[],
): boolean[] {
  return buildOpponentFastEditPlan(sequenceItems).hardCutAfter;
}

function toFastEditClip(item: EditSequenceItem, index: number): FastEditClip | undefined {
  const history = item.historyItem;
  if (String(history.history_type || "produce_clip").trim().toLowerCase() !== "produce_clip") {
    return undefined;
  }
  if (String(history.view || "").trim().toLowerCase() !== "victim") return undefined;
  const duration = Number(item.duration);
  if (!Number.isFinite(duration) || duration <= 0) return undefined;
  if (duration < OPPONENT_FAST_EDIT_SAFE_MIN_SECONDS) return undefined;

  const rate = Number(history.tick_rate);
  const recordStart = Number(history.record_start_tick);
  const recordEnd = Number(history.record_end_tick);
  if (!Number.isFinite(rate) || rate <= 0 || !Number.isFinite(recordStart) || recordStart < 0) {
    return undefined;
  }
  if (!Number.isFinite(recordEnd) || recordEnd <= recordStart) return undefined;

  const events = resolveEvents(history, duration, rate, recordStart, recordEnd);
  if (!events.length) return undefined;
  const groupKey = fastEditGroupKey(history);
  if (!groupKey) return undefined;

  return {
    index,
    duration,
    events,
    groupKey,
    recordBaseSeconds: recordStart / rate,
  };
}

function resolveEvents(
  history: ProduceHistoryItem,
  duration: number,
  rate: number,
  recordStart: number,
  recordEnd: number,
): FastEditEvent[] {
  const kills = Array.isArray(history.kills) ? history.kills : [];
  if (!kills.length) return [];
  const offsets = Array.isArray(history.kill_offsets_seconds)
    ? history.kill_offsets_seconds.map((value) => Number(value))
    : [];
  if (offsets.length !== kills.length && offsets.length > 0) return [];

  const events: FastEditEvent[] = [];
  for (let index = 0; index < kills.length; index++) {
    const kill = kills[index];
    const tick = Number(kill.tick);
    if (!Number.isFinite(tick) || tick < recordStart || tick > recordEnd) return [];
    const derivedOffset = (tick - recordStart) / rate;
    const offsetSeconds = offsets.length ? offsets[index] : derivedOffset;
    if (!Number.isFinite(offsetSeconds) || offsetSeconds < 0 || offsetSeconds > duration) {
      return [];
    }
    events.push({ offsetSeconds });
  }
  return events.sort((left, right) => left.offsetSeconds - right.offsetSeconds);
}

function fastEditGroupKey(history: ProduceHistoryItem): string {
  const kills = Array.isArray(history.kills) ? history.kills : [];
  const killers = kills.length
    ? Array.from(
        new Set(
          kills
            .map((kill) => {
              const steamID = String(kill.killer_steam_id || "").trim();
              if (steamID && steamID !== "0") return `steam:${steamID}`;
              const name = String(kill.killer_name || "").trim().toLowerCase();
              return name ? `name:${name}` : "";
            })
        ),
      )
    : [];
  if (killers.length !== 1 || killers[0] === "") return "";
  const demoPath = String(history.demo_path || "").trim();
  if (!demoPath) return "";
  const historyRound = Number(history.round);
  const killRound = Number(killsRound(history));
  const round = Number.isFinite(historyRound) && historyRound > 0 ? historyRound : killRound;
  if (!Number.isFinite(round) || round <= 0) return "";
  if (
    kills.some((kill) => {
      const killRound = Number(kill.round);
      return Number.isFinite(killRound) && killRound > 0 && killRound !== round;
    })
  ) {
    return "";
  }
  return [
    demoPath,
    String(round),
    killers[0],
  ].join("\u001f");
}

function killsRound(history: ProduceHistoryItem): number {
  if (!Array.isArray(history.kills) || history.kills.length === 0) return Number.NaN;
  const rounds = new Set(
    history.kills
      .map((kill) => Number(kill.round))
      .filter((round) => Number.isFinite(round) && round > 0),
  );
  return rounds.size === 1 ? [...rounds][0] : Number.NaN;
}

function absoluteBoundary(
  clip: FastEditClip,
  range: EditTrimRange | undefined,
): number | undefined {
  if (!range) return undefined;
  return clip.recordBaseSeconds + range.end_seconds;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

function roundSeconds(value: number, upperBound: number): number {
  const rounded = Math.round(value * 1000) / 1000;
  return Math.max(0, Math.min(upperBound, rounded));
}
