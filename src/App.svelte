<script lang="ts">
  import { onMount } from 'svelte';
  import { events, connectEvents, sendDelta } from './lib/bus';

  const qs = new URLSearchParams(location.search);
  const base = qs.get('node') ?? '';

  let es: EventSource | null = null;
  let inputValue = '';
  let group = 'demo';
  let key = '';

  function mkKey() {
    return `k-${Date.now().toString(36)}-${Math.random().toString(36).slice(2,6)}`;
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (!inputValue.trim()) return;

    const payload = {
      group,
      key: key || mkKey(),
      value: inputValue.trim(),
    };

    try {
      await sendDelta(base, payload);
      inputValue = '';
    } catch (err) {
      alert((err as Error).message);
    }
  }

  onMount(() => {
    es = connectEvents(base);
    return () => es?.close();
  });
</script>

<div class="wrap">
  <!-- TOP: input -->
  <div class="pane">
    <div class="title">Create delta</div>
    <form on:submit|preventDefault={submit} class="form-grid">
      <input placeholder="value…" bind:value={inputValue} />
      <input placeholder="group (optional)" bind:value={group} />
      <input placeholder="key (optional; auto if blank)" bind:value={key} />
      <button disabled={!inputValue.trim()}>Send</button>
    </form>
    <div class="note mono">
      Posting to {base || '(same-origin)'} /api/delta
    </div>
  </div>

  <!-- BOTTOM: output box -->
  <div class="events" id="events">
    <div class="title" style="color: white;">Events</div>

    {#each [...$events].slice(-200).reverse() as e}
      {#if (e.type === 'delta_added' || e.type === 'delta_updated' || e.type === 'participant_added' || e.type === 'participant_dead') && e.payload.group !== 'failure'}
        <div class="line">
        <span class={"chip " + e.type.replaceAll("_", "-")}>
          {e.type.replaceAll("_", " ")}
        </span>

          {#if e.type.startsWith('delta')}
            <span class="mono dim">{e.payload.group}:{e.payload.key}</span>

            {#if e.payload.value !== undefined}
            <span>
              "{String(e.payload.value).replaceAll('\r', '').replaceAll('\n', '')}"
            </span>
            {/if}

            {#if e.payload.version}
              <span class="dim">v{e.payload.version}</span>
            {/if}

          {:else if e.type === 'participant_added'}
            <span class="mono">{e.payload.node}</span>
            <span class="dim">joined @ {e.payload.time}</span>

          {:else if e.type === 'participant_dead'}
            <span class="mono">{e.payload.node}</span>
            <span class="dim">marked dead @ {e.payload.time}</span>
            <span class="dim">{e.payload.address}</span>
          {/if}
        </div>
      {/if}
    {/each}
  </div>

</div>
