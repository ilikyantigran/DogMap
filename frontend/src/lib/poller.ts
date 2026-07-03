// A tiny interval-based poller. This is the seam that lets us swap polling for
// WebSocket/SSE later WITHOUT touching components (Docs/03-Frontend.md:
// "hide it behind a store action so it can be swapped ... later").
//
// Stores own Poller instances and call start()/stop(); components never poll.

export interface Poller {
  /** Start the interval. Optionally run the tick immediately once. */
  start(runImmediately?: boolean): void
  /** Stop the interval. Safe to call when already stopped. */
  stop(): void
  readonly isRunning: boolean
}

export function createPoller(
  tick: () => void | Promise<void>,
  intervalMs: number,
): Poller {
  let handle: ReturnType<typeof setInterval> | null = null
  let inFlight = false

  const runTick = async () => {
    // Skip overlapping ticks if a slow request is still in flight.
    if (inFlight) return
    inFlight = true
    try {
      await tick()
    } finally {
      inFlight = false
    }
  }

  return {
    start(runImmediately = false) {
      if (handle !== null) return
      handle = setInterval(runTick, intervalMs)
      if (runImmediately) void runTick()
    },
    stop() {
      if (handle !== null) {
        clearInterval(handle)
        handle = null
      }
    },
    get isRunning() {
      return handle !== null
    },
  }
}
