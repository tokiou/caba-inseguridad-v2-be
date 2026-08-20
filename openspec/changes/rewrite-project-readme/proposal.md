# Rewrite Project README

## Why

The previous README was written in Spanish and described the system with
generic scalability language. It did not communicate the implemented
architecture or the benchmark evidence already committed under `bench/`.

## What

Rewrite the root README in professional English. The document will explain:

- the implemented system and data flow;
- the architectural mechanisms used for horizontal scaling and bounded
  resource consumption;
- the benchmark methodology and measured p50/p95/p99, throughput, cache, and
  pool-saturation results;
- the distinction between measured behavior and future production work.

## Scope

Documentation only. No runtime behavior, benchmark scripts, or application
code will change.
