import type { ProduceHistoryItem } from "@/shared/types";

export type EditTransitionMode = "none" | "fade";

export interface EditSequenceItem {
  id: string;
  historyItem: ProduceHistoryItem;
  videoPath: string;
  duration: number;
}

export interface EditConcatClipPayload {
  video_path: string;
  duration: number;
}

export interface EditConcatTransitionPayload {
  type: "fade";
  duration: number;
  after_index: number;
}

export interface EditConcatRequestPayload {
  clips: EditConcatClipPayload[];
  transitions: EditConcatTransitionPayload[];
}
