<script lang="ts">
  import { tick } from 'svelte';
  import { m } from '$lib/i18n';
  import { policyDevicesStore } from '$lib/stores/routing';
  import { Toggle, Button } from '$lib/components/ui';
  import type { SingboxRouterSettings } from '$lib/types';

  let { cfg, onPatch }: {
    cfg: SingboxRouterSettings;
    onPatch: (patch: Partial<SingboxRouterSettings>) => void | Promise<void>;
  } = $props();

  let query = $state('');
  let busy = $state(false);
  const selected = $derived((cfg.bypassDevices ?? []).map((mac) => mac.toLowerCase()));
  const rows = $derived.by(() => {
    const devices = new Map(
      ($policyDevicesStore.data ?? []).map((device) => [device.mac.toLowerCase(), {
        mac: device.mac.toLowerCase(),
        name: device.name || device.hostname || device.mac,
        ip: device.ip,
      }]),
    );
    // A saved exclusion remains removable when NDMS no longer lists the host.
    for (const mac of selected) {
      if (!devices.has(mac)) devices.set(mac, { mac, name: mac, ip: '' });
    }
    const q = query.trim().toLowerCase();
    return [...devices.values()].filter((device) =>
      `${device.name} ${device.mac} ${device.ip}`.toLowerCase().includes(q),
    );
  });

  async function toggle(mac: string, checked: boolean) {
    if (busy) return;
    busy = true;
    try {
      await onPatch({
        bypassDevices: checked ? [...selected, mac] : selected.filter((value) => value !== mac),
      });
      await tick();
    } finally {
      busy = false;
    }
  }
</script>

<section class="sec">
  <h3>{m.sb_router_bypass_devices_title()}</h3>
  <p class="hint">{m.sb_router_bypass_devices_hint()}</p>
  <input
    type="search"
    class="search"
    bind:value={query}
    aria-label={m.sb_router_bypass_devices_search()}
    placeholder={m.sb_router_bypass_devices_search()}
  />
  {#if $policyDevicesStore.error}
    <p class="hint" role="alert">{m.sb_router_bypass_devices_load_failed()}</p>
    <Button variant="ghost" size="sm" onclick={() => void policyDevicesStore.refetch()}>{m.common_retry()}</Button>
  {/if}
  <div class="devices" aria-busy={busy}>
    {#each rows as device (device.mac)}
      <div class="device">
        <div class="info">
          <span class="name">{device.name}</span>
          <span class="address">{device.ip} {device.mac}</span>
        </div>
        <Toggle
          checked={selected.includes(device.mac)}
          onchange={(checked) => void toggle(device.mac, checked)}
          disabled={busy}
          controlled
          size="sm"
          ariaLabel={m.sb_router_bypass_devices_toggle({ name: device.name })}
        />
      </div>
    {:else}
      <p class="hint">{!$policyDevicesStore.data && ($policyDevicesStore.status === 'idle' || $policyDevicesStore.status === 'loading')
        ? m.common_loading() : m.sb_router_bypass_devices_empty()}</p>
    {/each}
  </div>
</section>

<style>
  .sec { padding: 14px var(--sp-4); border-bottom: 1px solid var(--border); display: flex; flex-direction: column; gap: 10px; }
  h3 { margin: 0; font-size: 11px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-muted); }
  .hint { margin: 0; font-size: 11.5px; color: var(--text-muted); line-height: 1.4; }
  .search { width: 100%; min-width: 0; padding: 8px 10px; border: 1px solid var(--border); border-radius: var(--radius-sm, 6px); background: var(--bg-secondary); color: var(--text-primary); font: inherit; font-size: 12px; }
  .devices { max-height: 280px; overflow-y: auto; }
  .device { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 8px 0; }
  .info { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
  .name { overflow-wrap: anywhere; color: var(--text-primary); font-size: 12px; }
  .address { overflow-wrap: anywhere; color: var(--text-muted); font: 10px var(--font-mono); }
</style>
