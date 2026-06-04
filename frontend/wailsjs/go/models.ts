export namespace app {
	
	export class ClipActionSettings {
	    enable_voice_indices: boolean;
	    voice_indices_value: number;
	    enable_voice_indices_h: boolean;
	    voice_indices_h_value: number;
	
	    static createFrom(source: any = {}) {
	        return new ClipActionSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable_voice_indices = source["enable_voice_indices"];
	        this.voice_indices_value = source["voice_indices_value"];
	        this.enable_voice_indices_h = source["enable_voice_indices_h"];
	        this.voice_indices_h_value = source["voice_indices_h_value"];
	    }
	}
	export class ClipItemOverrides {
	    killer_pre_seconds?: number;
	    killer_post_seconds?: number;
	    victim_pre_seconds?: number;
	    victim_post_seconds?: number;
	    enable_voice?: boolean;
	    enable_spec_show_xray_zero?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClipItemOverrides(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.killer_pre_seconds = source["killer_pre_seconds"];
	        this.killer_post_seconds = source["killer_post_seconds"];
	        this.victim_pre_seconds = source["victim_pre_seconds"];
	        this.victim_post_seconds = source["victim_post_seconds"];
	        this.enable_voice = source["enable_voice"];
	        this.enable_spec_show_xray_zero = source["enable_spec_show_xray_zero"];
	    }
	}
	export class ClipSettings {
	    killer_pre_seconds: number;
	    killer_post_seconds: number;
	    victim_pre_seconds: number;
	    victim_post_seconds: number;
	    auto_add_victim_view: boolean;
	    enable_voice: boolean;
	    record_fps: number;
	    record_quality: string;
	    edit_fps: number;
	    edit_quality: string;
	    video_preset: string;
	    launch_resolution: string;
	    record_output_dir: string;
	    enable_spec_show_xray_zero: boolean;
	    hide_all_ui: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClipSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.killer_pre_seconds = source["killer_pre_seconds"];
	        this.killer_post_seconds = source["killer_post_seconds"];
	        this.victim_pre_seconds = source["victim_pre_seconds"];
	        this.victim_post_seconds = source["victim_post_seconds"];
	        this.auto_add_victim_view = source["auto_add_victim_view"];
	        this.enable_voice = source["enable_voice"];
	        this.record_fps = source["record_fps"];
	        this.record_quality = source["record_quality"];
	        this.edit_fps = source["edit_fps"];
	        this.edit_quality = source["edit_quality"];
	        this.video_preset = source["video_preset"];
	        this.launch_resolution = source["launch_resolution"];
	        this.record_output_dir = source["record_output_dir"];
	        this.enable_spec_show_xray_zero = source["enable_spec_show_xray_zero"];
	        this.hide_all_ui = source["hide_all_ui"];
	    }
	}
	export class DemoStorageStats {
	    demo_dir: string;
	    demo_count: number;
	    total_size_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new DemoStorageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_dir = source["demo_dir"];
	        this.demo_count = source["demo_count"];
	        this.total_size_bytes = source["total_size_bytes"];
	    }
	}
	export class EditConcatClip {
	    video_path: string;
	    duration: number;
	
	    static createFrom(source: any = {}) {
	        return new EditConcatClip(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.video_path = source["video_path"];
	        this.duration = source["duration"];
	    }
	}
	export class EditConcatTransition {
	    type: string;
	    duration: number;
	    after_index?: number;
	
	    static createFrom(source: any = {}) {
	        return new EditConcatTransition(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.duration = source["duration"];
	        this.after_index = source["after_index"];
	    }
	}
	export class EditConcatRequest {
	    clips: EditConcatClip[];
	    transitions: EditConcatTransition[];
	
	    static createFrom(source: any = {}) {
	        return new EditConcatRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.clips = this.convertValues(source["clips"], EditConcatClip);
	        this.transitions = this.convertValues(source["transitions"], EditConcatTransition);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ExportProduceHistoryResult {
	    cancelled: boolean;
	    target_dir?: string;
	    total: number;
	    moved: number;
	    failed: number;
	    errors?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ExportProduceHistoryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cancelled = source["cancelled"];
	        this.target_dir = source["target_dir"];
	        this.total = source["total"];
	        this.moved = source["moved"];
	        this.failed = source["failed"];
	        this.errors = source["errors"];
	    }
	}
	export class GameInfoStatus {
	    status: string;
	    clean: boolean;
	    has_backup: boolean;
	    can_repair: boolean;
	    gameinfo_path?: string;
	    backup_path?: string;
	    message?: string;
	
	    static createFrom(source: any = {}) {
	        return new GameInfoStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.clean = source["clean"];
	        this.has_backup = source["has_backup"];
	        this.can_repair = source["can_repair"];
	        this.gameinfo_path = source["gameinfo_path"];
	        this.backup_path = source["backup_path"];
	        this.message = source["message"];
	    }
	}
	export class GeneratePluginJSONBatchDebug {
	    keep_intermediate_files: boolean;
	
	    static createFrom(source: any = {}) {
	        return new GeneratePluginJSONBatchDebug(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.keep_intermediate_files = source["keep_intermediate_files"];
	    }
	}
	export class ProduceTakePlan {
	    demo_path: string;
	    take_index: number;
	    take_name?: string;
	    view: string;
	    spec_mode: number;
	    kill_ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new ProduceTakePlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_path = source["demo_path"];
	        this.take_index = source["take_index"];
	        this.take_name = source["take_name"];
	        this.view = source["view"];
	        this.spec_mode = source["spec_mode"];
	        this.kill_ids = source["kill_ids"];
	    }
	}
	export class GeneratePluginJSONBatchItemResult {
	    demo_path: string;
	    json_path?: string;
	    sequence_count?: number;
	    segment_count?: number;
	    action_count?: number;
	    take_plans?: ProduceTakePlan[];
	    generated_take_count?: number;
	    skipped_by_history?: boolean;
	    skipped_reason?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GeneratePluginJSONBatchItemResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_path = source["demo_path"];
	        this.json_path = source["json_path"];
	        this.sequence_count = source["sequence_count"];
	        this.segment_count = source["segment_count"];
	        this.action_count = source["action_count"];
	        this.take_plans = this.convertValues(source["take_plans"], ProduceTakePlan);
	        this.generated_take_count = source["generated_take_count"];
	        this.skipped_by_history = source["skipped_by_history"];
	        this.skipped_reason = source["skipped_reason"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SelectedClipItem {
	    kill: demo.ClipKill;
	    include_victim: boolean;
	    killer_spec_mode: number;
	    victim_spec_mode: number;
	    clip_overrides?: ClipItemOverrides;
	
	    static createFrom(source: any = {}) {
	        return new SelectedClipItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.kill = this.convertValues(source["kill"], demo.ClipKill);
	        this.include_victim = source["include_victim"];
	        this.killer_spec_mode = source["killer_spec_mode"];
	        this.victim_spec_mode = source["victim_spec_mode"];
	        this.clip_overrides = this.convertValues(source["clip_overrides"], ClipItemOverrides);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GeneratePluginJSONRequest {
	    demo_path: string;
	    tick_rate: number;
	    selected_items?: SelectedClipItem[];
	    extra_commands?: string[];
	    batch_timestamp?: string;
	    selected_kills?: demo.ClipKill[];
	
	    static createFrom(source: any = {}) {
	        return new GeneratePluginJSONRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_path = source["demo_path"];
	        this.tick_rate = source["tick_rate"];
	        this.selected_items = this.convertValues(source["selected_items"], SelectedClipItem);
	        this.extra_commands = source["extra_commands"];
	        this.batch_timestamp = source["batch_timestamp"];
	        this.selected_kills = this.convertValues(source["selected_kills"], demo.ClipKill);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GeneratePluginJSONBatchRequest {
	    jobs: GeneratePluginJSONRequest[];
	    debug?: GeneratePluginJSONBatchDebug;
	
	    static createFrom(source: any = {}) {
	        return new GeneratePluginJSONBatchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.jobs = this.convertValues(source["jobs"], GeneratePluginJSONRequest);
	        this.debug = this.convertValues(source["debug"], GeneratePluginJSONBatchDebug);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GeneratePluginJSONBatchResult {
	    results: GeneratePluginJSONBatchItemResult[];
	    success_count: number;
	    failure_count: number;
	    batch_timestamp?: string;
	    launch_started?: boolean;
	    launched_demo_path?: string;
	    launch_error?: string;
	
	    static createFrom(source: any = {}) {
	        return new GeneratePluginJSONBatchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.results = this.convertValues(source["results"], GeneratePluginJSONBatchItemResult);
	        this.success_count = source["success_count"];
	        this.failure_count = source["failure_count"];
	        this.batch_timestamp = source["batch_timestamp"];
	        this.launch_started = source["launch_started"];
	        this.launched_demo_path = source["launched_demo_path"];
	        this.launch_error = source["launch_error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class GeneratePluginJSONResult {
	    json_path: string;
	    sequence_count: number;
	    segment_count: number;
	    action_count: number;
	    take_plans?: ProduceTakePlan[];
	
	    static createFrom(source: any = {}) {
	        return new GeneratePluginJSONResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.json_path = source["json_path"];
	        this.sequence_count = source["sequence_count"];
	        this.segment_count = source["segment_count"];
	        this.action_count = source["action_count"];
	        this.take_plans = this.convertValues(source["take_plans"], ProduceTakePlan);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class OutputsStorageStats {
	    output_dir: string;
	    video_count: number;
	    total_size_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new OutputsStorageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.output_dir = source["output_dir"];
	        this.video_count = source["video_count"];
	        this.total_size_bytes = source["total_size_bytes"];
	    }
	}
	export class PlatformClientCloseResult {
	    exe_name: string;
	    closed: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlatformClientCloseResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exe_name = source["exe_name"];
	        this.closed = source["closed"];
	        this.error = source["error"];
	    }
	}
	export class PlatformClientStatus {
	    exe_name: string;
	    display_name: string;
	    running: boolean;
	    pid: number;
	
	    static createFrom(source: any = {}) {
	        return new PlatformClientStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exe_name = source["exe_name"];
	        this.display_name = source["display_name"];
	        this.running = source["running"];
	        this.pid = source["pid"];
	    }
	}
	export class ProduceHistoryItem {
	    demo_path: string;
	    take_index: number;
	    take_name?: string;
	    view: string;
	    spec_mode: number;
	    kill_ids: string[];
	    kills?: demo.ClipKill[];
	    video_path: string;
	    history_type?: string;
	    source_label?: string;
	    completed_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ProduceHistoryItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_path = source["demo_path"];
	        this.take_index = source["take_index"];
	        this.take_name = source["take_name"];
	        this.view = source["view"];
	        this.spec_mode = source["spec_mode"];
	        this.kill_ids = source["kill_ids"];
	        this.kills = this.convertValues(source["kills"], demo.ClipKill);
	        this.video_path = source["video_path"];
	        this.history_type = source["history_type"];
	        this.source_label = source["source_label"];
	        this.completed_at_ms = source["completed_at_ms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProduceHistorySnapshot {
	    items: ProduceHistoryItem[];
	    updated_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ProduceHistorySnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ProduceHistoryItem);
	        this.updated_at_ms = source["updated_at_ms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProduceTakeFile {
	    demo_path: string;
	    take_index: number;
	    take_name?: string;
	    view: string;
	    video_path?: string;
	    audio_path?: string;
	    status: string;
	    error?: string;
	    updated_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ProduceTakeFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_path = source["demo_path"];
	        this.take_index = source["take_index"];
	        this.take_name = source["take_name"];
	        this.view = source["view"];
	        this.video_path = source["video_path"];
	        this.audio_path = source["audio_path"];
	        this.status = source["status"];
	        this.error = source["error"];
	        this.updated_at_ms = source["updated_at_ms"];
	    }
	}
	export class ProduceTakeFileSnapshot {
	    items: ProduceTakeFile[];
	    updated_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new ProduceTakeFileSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ProduceTakeFile);
	        this.updated_at_ms = source["updated_at_ms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class WorkspaceState {
	    initialized: boolean;
	    data_dir: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.initialized = source["initialized"];
	        this.data_dir = source["data_dir"];
	        this.error = source["error"];
	    }
	}
	export class WorkspaceValidateResult {
	    ok: boolean;
	    errorMessage: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceValidateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.errorMessage = source["errorMessage"];
	    }
	}

}

export namespace config {
	
	export class ClipActionSettings {
	    enable_voice_indices: boolean;
	    voice_indices_value: number;
	    enable_voice_indices_h: boolean;
	    voice_indices_h_value: number;
	
	    static createFrom(source: any = {}) {
	        return new ClipActionSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enable_voice_indices = source["enable_voice_indices"];
	        this.voice_indices_value = source["voice_indices_value"];
	        this.enable_voice_indices_h = source["enable_voice_indices_h"];
	        this.voice_indices_h_value = source["voice_indices_h_value"];
	    }
	}
	export class Config {
	    cs2_dir: string;
	    cs2_exe: string;
	    hlae_exe: string;
	    plugin_dll: string;
	    ffmpeg_dir: string;
	    fivee_player_name: string;
	    download_source: string;
	    country_code: string;
	    source_checked_at: string;
	    killer_pre_seconds: number;
	    killer_post_seconds: number;
	    victim_pre_seconds: number;
	    victim_post_seconds: number;
	    auto_add_victim_view: boolean;
	    record_fps: number;
	    record_quality: string;
	    edit_fps: number;
	    edit_quality: string;
	    video_preset: string;
	    ffmpeg_detected_preset?: string;
	    ffmpeg_detected_encoders?: string[];
	    ffmpeg_detected_at?: string;
	    launch_resolution: string;
	    record_output_dir: string;
	    enable_spec_show_xray_zero: boolean;
	    hide_all_ui: boolean;
	    clip_action_settings?: ClipActionSettings;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cs2_dir = source["cs2_dir"];
	        this.cs2_exe = source["cs2_exe"];
	        this.hlae_exe = source["hlae_exe"];
	        this.plugin_dll = source["plugin_dll"];
	        this.ffmpeg_dir = source["ffmpeg_dir"];
	        this.fivee_player_name = source["fivee_player_name"];
	        this.download_source = source["download_source"];
	        this.country_code = source["country_code"];
	        this.source_checked_at = source["source_checked_at"];
	        this.killer_pre_seconds = source["killer_pre_seconds"];
	        this.killer_post_seconds = source["killer_post_seconds"];
	        this.victim_pre_seconds = source["victim_pre_seconds"];
	        this.victim_post_seconds = source["victim_post_seconds"];
	        this.auto_add_victim_view = source["auto_add_victim_view"];
	        this.record_fps = source["record_fps"];
	        this.record_quality = source["record_quality"];
	        this.edit_fps = source["edit_fps"];
	        this.edit_quality = source["edit_quality"];
	        this.video_preset = source["video_preset"];
	        this.ffmpeg_detected_preset = source["ffmpeg_detected_preset"];
	        this.ffmpeg_detected_encoders = source["ffmpeg_detected_encoders"];
	        this.ffmpeg_detected_at = source["ffmpeg_detected_at"];
	        this.launch_resolution = source["launch_resolution"];
	        this.record_output_dir = source["record_output_dir"];
	        this.enable_spec_show_xray_zero = source["enable_spec_show_xray_zero"];
	        this.hide_all_ui = source["hide_all_ui"];
	        this.clip_action_settings = this.convertValues(source["clip_action_settings"], ClipActionSettings);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace demo {
	
	export class ClipKill {
	    id: string;
	    round: number;
	    tick: number;
	    map_name: string;
	    killer_name: string;
	    killer_steam_id: string;
	    killer_slot: number;
	    killer_entity_id: number;
	    killer_side: string;
	    victim_name: string;
	    victim_steam_id: string;
	    victim_slot: number;
	    victim_entity_id: number;
	    victim_side: string;
	    weapon_name: string;
	    is_headshot: boolean;
	    is_wallbang: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ClipKill(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.round = source["round"];
	        this.tick = source["tick"];
	        this.map_name = source["map_name"];
	        this.killer_name = source["killer_name"];
	        this.killer_steam_id = source["killer_steam_id"];
	        this.killer_slot = source["killer_slot"];
	        this.killer_entity_id = source["killer_entity_id"];
	        this.killer_side = source["killer_side"];
	        this.victim_name = source["victim_name"];
	        this.victim_steam_id = source["victim_steam_id"];
	        this.victim_slot = source["victim_slot"];
	        this.victim_entity_id = source["victim_entity_id"];
	        this.victim_side = source["victim_side"];
	        this.weapon_name = source["weapon_name"];
	        this.is_headshot = source["is_headshot"];
	        this.is_wallbang = source["is_wallbang"];
	    }
	}
	export class ClipRound {
	    round: number;
	    kills: ClipKill[];
	
	    static createFrom(source: any = {}) {
	        return new ClipRound(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.round = source["round"];
	        this.kills = this.convertValues(source["kills"], ClipKill);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ClipPlayer {
	    name: string;
	    steam_id: string;
	    total_kills: number;
	    rounds: ClipRound[];
	
	    static createFrom(source: any = {}) {
	        return new ClipPlayer(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.steam_id = source["steam_id"];
	        this.total_kills = source["total_kills"];
	        this.rounds = this.convertValues(source["rounds"], ClipRound);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PlayerInfo {
	    name: string;
	    steam_id: number;
	    kills: number;
	    deaths: number;
	    assists: number;
	
	    static createFrom(source: any = {}) {
	        return new PlayerInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.steam_id = source["steam_id"];
	        this.kills = source["kills"];
	        this.deaths = source["deaths"];
	        this.assists = source["assists"];
	    }
	}
	export class Metadata {
	    file_path: string;
	    file_name: string;
	    map_name: string;
	    server_name: string;
	    duration: number;
	    tick_rate: number;
	    total_rounds: number;
	    overtime_count: number;
	    score_ct: number;
	    score_t: number;
	    clan_name_ct: string;
	    clan_name_t: string;
	    players: PlayerInfo[];
	    clip_players: ClipPlayer[];
	
	    static createFrom(source: any = {}) {
	        return new Metadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file_path = source["file_path"];
	        this.file_name = source["file_name"];
	        this.map_name = source["map_name"];
	        this.server_name = source["server_name"];
	        this.duration = source["duration"];
	        this.tick_rate = source["tick_rate"];
	        this.total_rounds = source["total_rounds"];
	        this.overtime_count = source["overtime_count"];
	        this.score_ct = source["score_ct"];
	        this.score_t = source["score_t"];
	        this.clan_name_ct = source["clan_name_ct"];
	        this.clan_name_t = source["clan_name_t"];
	        this.players = this.convertValues(source["players"], PlayerInfo);
	        this.clip_players = this.convertValues(source["clip_players"], ClipPlayer);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace envsetup {
	
	export class ComponentStatus {
	    id: string;
	    name: string;
	    status: string;
	    local_version: string;
	    remote_version: string;
	    path: string;
	    error: string;
	    manual_url: string;
	
	    static createFrom(source: any = {}) {
	        return new ComponentStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.status = source["status"];
	        this.local_version = source["local_version"];
	        this.remote_version = source["remote_version"];
	        this.path = source["path"];
	        this.error = source["error"];
	        this.manual_url = source["manual_url"];
	    }
	}
	export class SelfUpdateState {
	    status: string;
	    available: boolean;
	    current: string;
	    latest: string;
	    url: string;
	    asset_url: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SelfUpdateState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.available = source["available"];
	        this.current = source["current"];
	        this.latest = source["latest"];
	        this.url = source["url"];
	        this.asset_url = source["asset_url"];
	        this.error = source["error"];
	    }
	}
	export class SourceStepState {
	    status: string;
	    source: string;
	    country_code: string;
	    message: string;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new SourceStepState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.source = source["source"];
	        this.country_code = source["country_code"];
	        this.message = source["message"];
	        this.error = source["error"];
	    }
	}
	export class StartupAd {
	    id: string;
	    enabled: boolean;
	    placement: string;
	    click_url: string;
	    sponsor: string;
	    title: string;
	    rich_html: string;
	    image_url: string;
	    image_alt?: string;
	
	    static createFrom(source: any = {}) {
	        return new StartupAd(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.enabled = source["enabled"];
	        this.placement = source["placement"];
	        this.click_url = source["click_url"];
	        this.sponsor = source["sponsor"];
	        this.title = source["title"];
	        this.rich_html = source["rich_html"];
	        this.image_url = source["image_url"];
	        this.image_alt = source["image_alt"];
	    }
	}
	export class StartupState {
	    mode: string;
	    phase: string;
	    running: boolean;
	    source_step: SourceStepState;
	    fatal_error: string;
	    entry_notice: string;
	    ads: StartupAd[];
	    self_update: SelfUpdateState;
	    steps: ComponentStatus[];
	    can_enter_main: boolean;
	    config: config.Config;
	
	    static createFrom(source: any = {}) {
	        return new StartupState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.phase = source["phase"];
	        this.running = source["running"];
	        this.source_step = this.convertValues(source["source_step"], SourceStepState);
	        this.fatal_error = source["fatal_error"];
	        this.entry_notice = source["entry_notice"];
	        this.ads = this.convertValues(source["ads"], StartupAd);
	        this.self_update = this.convertValues(source["self_update"], SelfUpdateState);
	        this.steps = this.convertValues(source["steps"], ComponentStatus);
	        this.can_enter_main = source["can_enter_main"];
	        this.config = this.convertValues(source["config"], config.Config);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace fivee {
	
	export class FiveEMatchItem {
	    match_id: string;
	    download_match_id: string;
	    map_name: string;
	    score1: number;
	    score2: number;
	    kill: number;
	    death: number;
	    assist: number;
	    rating: number;
	    end_time: string;
	
	    static createFrom(source: any = {}) {
	        return new FiveEMatchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.match_id = source["match_id"];
	        this.download_match_id = source["download_match_id"];
	        this.map_name = source["map_name"];
	        this.score1 = source["score1"];
	        this.score2 = source["score2"];
	        this.kill = source["kill"];
	        this.death = source["death"];
	        this.assist = source["assist"];
	        this.rating = source["rating"];
	        this.end_time = source["end_time"];
	    }
	}
	export class FiveEMatchListResult {
	    player_name: string;
	    matches: FiveEMatchItem[];
	
	    static createFrom(source: any = {}) {
	        return new FiveEMatchListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.player_name = source["player_name"];
	        this.matches = this.convertValues(source["matches"], FiveEMatchItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace producews {
	
	export class QueueState {
	    running: boolean;
	    total: number;
	    completed: number;
	    current_index: number;
	    current_demo_path?: string;
	    pending_ack: boolean;
	    last_error?: string;
	    demos?: string[];
	    updated_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new QueueState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.total = source["total"];
	        this.completed = source["completed"];
	        this.current_index = source["current_index"];
	        this.current_demo_path = source["current_demo_path"];
	        this.pending_ack = source["pending_ack"];
	        this.last_error = source["last_error"];
	        this.demos = source["demos"];
	        this.updated_at_ms = source["updated_at_ms"];
	    }
	}
	export class TakeStatus {
	    demo_path?: string;
	    take_index?: number;
	    take_name?: string;
	    record_phase?: string;
	    status: string;
	    tick?: number;
	    cmd?: string;
	    ts_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new TakeStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.demo_path = source["demo_path"];
	        this.take_index = source["take_index"];
	        this.take_name = source["take_name"];
	        this.record_phase = source["record_phase"];
	        this.status = source["status"];
	        this.tick = source["tick"];
	        this.cmd = source["cmd"];
	        this.ts_ms = source["ts_ms"];
	    }
	}
	export class TakeStatusSnapshot {
	    items: TakeStatus[];
	    total_takes: number;
	    started_takes: number;
	    completed_takes: number;
	    last_event?: TakeStatus;
	    updated_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new TakeStatusSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], TakeStatus);
	        this.total_takes = source["total_takes"];
	        this.started_takes = source["started_takes"];
	        this.completed_takes = source["completed_takes"];
	        this.last_event = this.convertValues(source["last_event"], TakeStatus);
	        this.updated_at_ms = source["updated_at_ms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class WSState {
	    address: string;
	    connected: boolean;
	    last_error?: string;
	    updated_at_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new WSState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.address = source["address"];
	        this.connected = source["connected"];
	        this.last_error = source["last_error"];
	        this.updated_at_ms = source["updated_at_ms"];
	    }
	}

}

export namespace wanmei {
	
	export class WanmeiMatchItem {
	    match_id: string;
	    download_match_id: string;
	    map_name: string;
	    score1: number;
	    score2: number;
	    kill: number;
	    death: number;
	    assist: number;
	    k4: number;
	    k5: number;
	    rating: number;
	    end_time: string;
	
	    static createFrom(source: any = {}) {
	        return new WanmeiMatchItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.match_id = source["match_id"];
	        this.download_match_id = source["download_match_id"];
	        this.map_name = source["map_name"];
	        this.score1 = source["score1"];
	        this.score2 = source["score2"];
	        this.kill = source["kill"];
	        this.death = source["death"];
	        this.assist = source["assist"];
	        this.k4 = source["k4"];
	        this.k5 = source["k5"];
	        this.rating = source["rating"];
	        this.end_time = source["end_time"];
	    }
	}
	export class WanmeiMatchListResult {
	    status: string;
	    nickname?: string;
	    steam_id?: string;
	    matches: WanmeiMatchItem[];
	
	    static createFrom(source: any = {}) {
	        return new WanmeiMatchListResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.nickname = source["nickname"];
	        this.steam_id = source["steam_id"];
	        this.matches = this.convertValues(source["matches"], WanmeiMatchItem);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

