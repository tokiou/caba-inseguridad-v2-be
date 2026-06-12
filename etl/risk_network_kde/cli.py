"""CLI for the Network Temporal KDE risk pipeline.

Run from the repo root, e.g.:
    python -m etl.risk_network_kde.cli snap-crimes --graph-version caba_walking_graph_v1 --max-distance-meters 80
"""

from __future__ import annotations

import argparse


def main() -> None:
    parser = argparse.ArgumentParser(prog="risk_network_kde")
    sub = parser.add_subparsers(dest="command", required=True)

    p = sub.add_parser("snap-crimes", help="snap crimes to the nearest routable edge")
    p.add_argument("--graph-version", required=True)
    p.add_argument("--max-distance-meters", type=float, default=80)

    p = sub.add_parser("build-neighborhoods", help="precompute edge network neighborhoods")
    p.add_argument("--graph-version", required=True)
    p.add_argument("--bandwidth-meters", type=float, default=350)

    p = sub.add_parser("compute-scores", help="compute edge risk scores for a model version")
    p.add_argument("--model", required=True)
    p.add_argument("--train-until", default=None)

    p = sub.add_parser("evaluate", help="temporal backtest against a held-out window")
    p.add_argument("--model", required=True)
    p.add_argument("--test-from", required=True)
    p.add_argument("--test-to", required=True)

    p = sub.add_parser("evaluate-routes", help="route-level simulation against the backtest labels")
    p.add_argument("--model", required=True)
    p.add_argument("--test-from", required=True)
    p.add_argument("--test-to", required=True)
    p.add_argument("--pairs", type=int, default=100)
    p.add_argument("--ksp-k", type=int, default=10)
    p.add_argument("--skip-least-safe", action="store_true")

    p = sub.add_parser("calibrate", help="grid-search candidates (run only if the gate fails)")
    p.add_argument("--base-model", default="network_temporal_edge_risk_v1_eval_2022")
    p.add_argument("--train-until", required=True)
    p.add_argument("--test-from", required=True)
    p.add_argument("--test-to", required=True)
    p.add_argument("--max-candidates", type=int, default=None)

    p = sub.add_parser("finalize", help="create + activate the production model")
    p.add_argument("--base-model", required=True)
    p.add_argument("--final-model", required=True)
    p.add_argument("--train-until", default="latest")
    p.add_argument("--activate", action="store_true")

    sub.add_parser("self-test", help="assert time-bucket/weight helpers")

    args = parser.parse_args()

    if args.command == "snap-crimes":
        from . import snap_crimes
        snap_crimes.run(args.graph_version, args.max_distance_meters)
    elif args.command == "build-neighborhoods":
        from . import build_neighborhoods
        build_neighborhoods.run(args.graph_version, args.bandwidth_meters)
    elif args.command == "compute-scores":
        from . import compute_scores
        compute_scores.run(args.model, args.train_until)
    elif args.command == "evaluate":
        from . import evaluate
        evaluate.run(args.model, args.test_from, args.test_to)
    elif args.command == "evaluate-routes":
        from . import evaluate_routes
        evaluate_routes.run(args.model, args.test_from, args.test_to,
                            pairs=args.pairs, ksp_k=args.ksp_k,
                            skip_least_safe=args.skip_least_safe)
    elif args.command == "calibrate":
        from . import calibrate
        calibrate.run(args.base_model, args.train_until, args.test_from, args.test_to,
                      max_candidates=args.max_candidates)
    elif args.command == "finalize":
        from . import finalize
        finalize.run(args.base_model, args.final_model,
                     train_until=args.train_until, activate=args.activate)
    elif args.command == "self-test":
        from . import utils
        utils.self_test()


if __name__ == "__main__":
    main()
