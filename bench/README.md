# bench/

Cross-language microbenchmark scripts.
The numbers feed [mvm.sh/bench](https://mvm.sh/bench) via `bench.json`.

## Workloads

- **fib35** -- recursive `fib(35)`, exercises function-call dispatch.
- **sieve** -- Eratosthenes sieve, N=10,000,000, exercises tight loops
  and array writes.

Each workload has one source file per runtime (`fib35.{go,py,lua}`,
`sieve.{go,py,lua}`).

## Reproducing locally

```
# install hyperfine if needed
make bench
```

The Makefile target builds `mvm`, runs each workload under
`mvm`, `lua5.4`, `lua5.1`, and `python3` via [hyperfine][hf], writes
`bench/bench.json`, and refreshes the `<!-- bench:start -->` block in
the repo's top-level README.

`bench.json` is a one-shot snapshot, not a time series.
Hardware varies a lot, so the numbers in the committed JSON reflect the
machine that last ran `make bench` (recorded in the `cpu` /
`platform` fields).
Re-run on your own machine for an apples-to-apples comparison.

[hf]: https://github.com/sharkdp/hyperfine

## Schema

`bench.json` is loaded by [bench/index.html in the site repo][page]:

```json
{
  "schemaVersion": 1,
  "generatedAt": "2026-05-26T13:30:00Z",
  "mvm": "abc1234",
  "platform": "linux/arm64",
  "cpu": "Apple M-class",
  "note": "...",
  "benchmarks": [
    {
      "name": "sieve",
      "description": "Eratosthenes sieve, N=10_000_000",
      "results": [
        {"runtime":"mvm","version":"abc1234","meanMs":334.4,"stddevMs":1.5},
        ...
      ]
    }
  ]
}
```

[page]: https://github.com/cogentcore/mvm-sh.github.io/blob/main/bench/index.html
