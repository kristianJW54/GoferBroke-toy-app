import { writable } from 'svelte/store';

export type Envelope = { type: string; payload: any };

// live event stream (raw envelopes so can render anything)
export const events = writable<Envelope[]>([]);

// optional: convenience view of just deltas
export const deltas = writable<any[]>([]);

export function connectEvents(base = '') {
    const es = new EventSource(`${base}/events`);

    es.onmessage = (e) => {
        try {
            const msg: Envelope = JSON.parse(e.data);
            events.update(a => (a.push(msg), a.length > 1000 && a.shift(), a));
            if (msg.type === 'delta' || msg.type === 'delta_update') {
                deltas.update(a => (a.push(msg.payload), a.length > 500 && a.shift(), a));
            }
        } catch (err) {
            console.warn('bad SSE payload', err);
        }
    };

    es.onerror = () => {
        console.warn('SSE error');
    };

    return es;
}

/** POST a new delta to this node’s API. Shape is up to your backend. */
export async function sendDelta(
    base: string,
    body: { value: string; group?: string; key?: string }
) {
    const res = await fetch(`${base}/api/delta`, {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(body),
    });
    if (!res.ok) throw new Error(`sendDelta failed: ${res.status}`);
    return res.json().catch(() => ({}));
}