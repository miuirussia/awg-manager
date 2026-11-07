import { beforeEach, describe, expect, it, vi } from 'vitest';
import '@testing-library/jest-dom/vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import type { SingboxRouterSettings } from '$lib/types';

const { devices } = await vi.hoisted(async () => {
  const { writable } = await import('svelte/store');
  return { devices: writable({
    data: [{ mac: 'AA:BB:CC:DD:EE:FF', name: 'Laptop', hostname: '', ip: '192.168.1.10' }],
    status: 'fresh', error: null,
  }) };
});

vi.mock('$lib/stores/routing', () => ({
  policyDevicesStore: { subscribe: devices.subscribe, refetch: vi.fn() },
}));

import BypassDevices from './BypassDevices.svelte';

describe('TProxy device bypass', () => {
  const cfg: SingboxRouterSettings = {
    enabled: true, policyName: 'p1', deviceMode: 'all', routingMode: 'tproxy',
    wanAutoDetect: true, snifferEnabled: false,
  };

  beforeEach(() => {
    devices.set({
      data: [{ mac: 'AA:BB:CC:DD:EE:FF', name: 'Laptop', hostname: '', ip: '192.168.1.10' }],
      status: 'fresh', error: null,
    });
  });

  it('saves the device MAC and keeps other exclusions', async () => {
    const onPatch = vi.fn().mockResolvedValue(undefined);
    render(BypassDevices, { cfg: { ...cfg, bypassDevices: ['02:11:22:33:44:55'] }, onPatch });
    await fireEvent.input(screen.getByRole('checkbox', { name: /Laptop/ }), { target: { checked: true } });
    expect(onPatch).toHaveBeenCalledWith({ bypassDevices: ['02:11:22:33:44:55', 'aa:bb:cc:dd:ee:ff'] });
  });

  it('allows removal of an excluded device missing from NDMS', async () => {
    const onPatch = vi.fn().mockResolvedValue(undefined);
    render(BypassDevices, { cfg: { ...cfg, bypassDevices: ['02:11:22:33:44:55'] }, onPatch });
    const toggle = screen.getByRole('checkbox', { name: /02:11:22:33:44:55/ });
    expect(toggle).toBeChecked();
    await fireEvent.input(toggle, { target: { checked: false } });
    expect(onPatch).toHaveBeenCalledWith({ bypassDevices: [] });
  });

  it('keeps the saved state when the parent handles a failed save', async () => {
    const onPatch = vi.fn().mockResolvedValue(undefined);
    render(BypassDevices, { cfg, onPatch });
    const toggle = screen.getByRole('checkbox', { name: /Laptop/ });
    await fireEvent.input(toggle, { target: { checked: true } });
    await waitFor(() => expect(toggle).not.toBeDisabled());
    expect(toggle).not.toBeChecked();
  });

  it('filters devices by IP and MAC', async () => {
    render(BypassDevices, { cfg, onPatch: vi.fn() });
    const search = screen.getByRole('searchbox');
    await fireEvent.input(search, { target: { value: '192.168.1.10' } });
    expect(screen.getByText('Laptop')).toBeInTheDocument();
    await fireEvent.input(search, { target: { value: 'aa:bb:cc' } });
    expect(screen.getByText('Laptop')).toBeInTheDocument();
    await fireEvent.input(search, { target: { value: 'absent' } });
    expect(screen.queryByText('Laptop')).not.toBeInTheDocument();
  });

  it('blocks overlapping saves', async () => {
    let finish!: () => void;
    const onPatch = vi.fn(() => new Promise<void>((resolve) => { finish = resolve; }));
    render(BypassDevices, { cfg, onPatch });
    const toggle = screen.getByRole('checkbox', { name: /Laptop/ });
    await fireEvent.input(toggle, { target: { checked: true } });
    expect(toggle).toBeDisabled();
    finish();
    await waitFor(() => expect(toggle).not.toBeDisabled());
  });
});
