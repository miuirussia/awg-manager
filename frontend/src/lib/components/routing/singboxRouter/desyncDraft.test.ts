import { describe, expect, it } from 'vitest';
import type { SingboxRouterDesyncFields } from '$lib/types';
import { desyncDraftFrom, desyncFieldsFrom, shortStrategy } from './desyncDraft';

describe('desyncDraft', () => {
	it('new outbound defaults to preset auto only', () => {
		expect(desyncFieldsFrom(desyncDraftFrom())).toEqual({ preset: 'auto' });
	});

	it('round-trips fields, keeps explicit auto_tune false, drops empties', () => {
		const src = {
			preset: 'fake_split',
			fake_sni: 'example.org',
			fake_sni_pool: ['a.ru', 'b.ru'],
			ttl: 4,
			fooling: 'badseq',
			auto_tune: false,
			adaptive: { max_attempts: 3, ttl: '24h' },
			strategy: 'seqovl=1',
		};
		expect(desyncFieldsFrom(desyncDraftFrom(src))).toEqual(src);
	});

	it('round-trips extended TCP, fake and QUIC settings including explicit false and zero', () => {
		const src: SingboxRouterDesyncFields = {
			preset: 'auto', mode: 'hostfakesplit', split_pos: 'sni+1,endhost-1',
			tlsrec_pos: 'midsld', segment_order: '2,1,3', tlsrec_first: true,
			fake_placement: 'before_between', fake_delay: '0s', fake_tls_random: false,
			fake_tls_size: 0, fake_tls_template: 'compact', http_mod: 'hcsmix,rmspace',
			hostfake_mode: 'legacy', autottl: false,
			udp_mode: 'auto', quic_desync: false, quic_repeats: 0,
			adaptive: { max_buffer: 32768, network_id: 'wan-1' },
		};
		expect(desyncFieldsFrom(desyncDraftFrom(src))).toEqual(src);
	});

	it('preserves QUIC CRYPTO positions and order', () => {
		const src = { udp_mode: 'quic', quic_desync: true, quic_repeats: 2,
			quic_split_pos: 'sni+1,endhost-1', quic_order: 'reverse' };
		expect(desyncFieldsFrom(desyncDraftFrom(src))).toEqual(src);
	});

	it('can explicitly disable TLS record splitting inherited from strategy', () => {
		const src = { strategy: 'tlsrec_first=true', tlsrec_first: false };
		expect(desyncFieldsFrom(desyncDraftFrom(src))).toEqual(src);
	});

	it.each(['fake_tls_size', 'quic_repeats', 'max_buffer'] as const)('rejects non-integer %s', (field) => {
		const d = desyncDraftFrom();
		d[field] = '1.5';
		expect(() => desyncFieldsFrom(d)).toThrow(field);
	});

	it('rejects non-integer numbers', () => {
		const d = desyncDraftFrom();
		d.ttl = '3.5';
		expect(() => desyncFieldsFrom(d)).toThrow('ttl');
	});
});

describe('shortStrategy', () => {
	it('укорачивает хеш и не трогает прочее', () => {
		expect(shortStrategy('multi_split:0123456789abcdef0123456789abcdef')).toBe('multi_split·012345');
		expect(shortStrategy('split')).toBe('split');
	});
});
