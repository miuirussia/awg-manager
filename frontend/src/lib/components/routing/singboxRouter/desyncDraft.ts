import type { SingboxRouterDesyncFields } from '$lib/types';

// Списки значений — из option/desync.go awgm-box awg-1.15.0-alpha.13. Проверку значений и
// сочетаний делает sing-box check при сохранении, форма её не дублирует.
export const DESYNC_PRESETS = ['auto', 'custom', 'split', 'disorder', 'tlsrec', 'fake', 'fake_split', 'oob', 'GT1', 'GT2', 'GT3', 'GT4'];
export const DESYNC_MODES = ['none', 'split', 'disorder', 'tlsrec', 'fake', 'fake_split', 'oob', 'multi_split', 'fake_disorder', 'multi_disorder', 'disoob', 'hostfakesplit'];
export const DESYNC_FOOLING = ['none', 'badseq', 'badack', 'badsum', 'md5', 'md5_badack', 'badseq_badack_badsum', 'datanoack', 'hopbyhop', 'destopt', 'ipfrag', 'ts', 'badtimestamp'];
export const DESYNC_UDP_MODES = ['none', 'quic', 'stun', 'auto'];

/** Состояние формы: всё строками, пустая строка = «не задано, решает пресет». */
type OptionalBool = '' | 'on' | 'off';

export interface DesyncDraft {
	preset: string;
	mode: string;
	split_pos: string;
	fake_sni: string;
	fake_sni_pool: string;
	fake_host: string;
	repeats: string;
	ttl: string;
	fooling: string;
	autottl: OptionalBool;
	tlsrec_first: OptionalBool;
	udp_mode: string;
	udp_ttl: string;
	udp_repeats: string;
	auto_tune: OptionalBool;
	max_attempts: string;
	max_concurrent: string;
	max_entries: string;
	adaptive_ttl: string;
	adaptive_timeout: string;
	tlsrec_pos: string;
	segment_order: string;
	fake_placement: string;
	fake_delay: string;
	fake_tls_random: OptionalBool;
	fake_tls_size: string;
	fake_tls_template: string;
	http_mod: string;
	hostfake_mode: string;
	quic_desync: OptionalBool;
	quic_repeats: string;
	quic_split_pos: string;
	quic_order: string;
	max_buffer: string;
	network_id: string;
	strategy: string;
}

/** Идентификатор стратегии Adaptive `режим:<32 hex>` → `режим·<6 hex>`: варианты режима различимы. */
export const shortStrategy = (id: string) => id.replace(/:([0-9a-f]{6})[0-9a-f]{26}$/, '·$1');

type DesyncSource = SingboxRouterDesyncFields & { strategy?: string };

const bool = (v: boolean | undefined): OptionalBool => v === undefined ? '' : v ? 'on' : 'off';

const str = (n: number | undefined) => (n === undefined ? '' : String(n));

export function desyncDraftFrom(o?: DesyncSource): DesyncDraft {
	const a = o?.adaptive ?? {};
	return {
		preset: o ? (o.preset ?? '') : 'auto',
		mode: o?.mode ?? '',
		split_pos: o?.split_pos ?? '',
		fake_sni: o?.fake_sni ?? '',
		fake_sni_pool: (o?.fake_sni_pool ?? []).join(', '),
		fake_host: o?.fake_host ?? '',
		repeats: str(o?.repeats),
		ttl: str(o?.ttl),
		fooling: o?.fooling ?? '',
		autottl: bool(o?.autottl),
		tlsrec_first: bool(o?.tlsrec_first),
		udp_mode: o?.udp_mode ?? '',
		udp_ttl: str(o?.udp_ttl),
		udp_repeats: str(o?.udp_repeats),
		auto_tune: bool(o?.auto_tune),
		max_attempts: str(a.max_attempts),
		max_concurrent: str(a.max_concurrent),
		max_entries: str(a.max_entries),
		adaptive_ttl: a.ttl ?? '',
		adaptive_timeout: a.timeout ?? '',
		tlsrec_pos: o?.tlsrec_pos ?? '',
		segment_order: o?.segment_order ?? '',
		fake_placement: o?.fake_placement ?? '',
		fake_delay: o?.fake_delay ?? '',
		fake_tls_random: bool(o?.fake_tls_random),
		fake_tls_size: str(o?.fake_tls_size),
		fake_tls_template: o?.fake_tls_template ?? '',
		http_mod: o?.http_mod ?? '',
		hostfake_mode: o?.hostfake_mode ?? '',
		quic_desync: bool(o?.quic_desync),
		quic_repeats: str(o?.quic_repeats),
		quic_split_pos: o?.quic_split_pos ?? '',
		quic_order: o?.quic_order ?? '',
		max_buffer: str(a.max_buffer),
		network_id: a.network_id ?? '',
		strategy: o?.strategy ?? '',
	};
}

/** Черновик → поля outbound'а; пустые не попадают. Бросает на нечисловом вводе. */
export function desyncFieldsFrom(d: DesyncDraft): DesyncSource {
	const out: DesyncSource = {};
	const text = (k: keyof DesyncSource & string, v: string) => {
		if (v.trim()) (out as Record<string, unknown>)[k] = v.trim();
	};
	const int = (name: string, v: string): number | undefined => {
		if (!v.trim()) return undefined;
		const n = Number(v.trim());
		if (!Number.isInteger(n)) throw new Error(`${name}: ${v}`);
		return n;
	};
	text('preset', d.preset);
	text('mode', d.mode);
	text('split_pos', d.split_pos);
	text('fake_sni', d.fake_sni);
	const pool = d.fake_sni_pool.split(',').map((s) => s.trim()).filter(Boolean);
	if (pool.length) out.fake_sni_pool = pool;
	text('fake_host', d.fake_host);
	out.repeats = int('repeats', d.repeats);
	out.ttl = int('ttl', d.ttl);
	text('fooling', d.fooling);
	if (d.autottl) out.autottl = d.autottl === 'on';
	if (d.tlsrec_first) out.tlsrec_first = d.tlsrec_first === 'on';
	text('udp_mode', d.udp_mode);
	out.udp_ttl = int('udp_ttl', d.udp_ttl);
	out.udp_repeats = int('udp_repeats', d.udp_repeats);
	if (d.auto_tune) out.auto_tune = d.auto_tune === 'on';
	text('tlsrec_pos', d.tlsrec_pos);
	text('segment_order', d.segment_order);
	text('fake_placement', d.fake_placement);
	text('fake_delay', d.fake_delay);
	if (d.fake_tls_random) out.fake_tls_random = d.fake_tls_random === 'on';
	out.fake_tls_size = int('fake_tls_size', d.fake_tls_size);
	text('fake_tls_template', d.fake_tls_template);
	text('http_mod', d.http_mod);
	text('hostfake_mode', d.hostfake_mode);
	if (d.quic_desync) out.quic_desync = d.quic_desync === 'on';
	out.quic_repeats = int('quic_repeats', d.quic_repeats);
	text('quic_split_pos', d.quic_split_pos);
	text('quic_order', d.quic_order);
	const adaptive = {
		max_attempts: int('max_attempts', d.max_attempts),
		max_concurrent: int('max_concurrent', d.max_concurrent),
		max_entries: int('max_entries', d.max_entries),
		ttl: d.adaptive_ttl.trim() || undefined,
		timeout: d.adaptive_timeout.trim() || undefined,
		max_buffer: int('max_buffer', d.max_buffer),
		network_id: d.network_id.trim() || undefined,
	};
	if (Object.values(adaptive).some((v) => v !== undefined)) out.adaptive = adaptive;
	text('strategy', d.strategy);
	// undefined при JSON.stringify выпадает сам; чистим и для сравнения черновиков.
	return JSON.parse(JSON.stringify(out));
}
