"""Time-bucket / weekday helpers shared by every pipeline stage.

The Go service (internal/saferoutes) mirrors these boundaries exactly — change
them in both places or not at all.
"""

from __future__ import annotations

import datetime

TIME_BUCKETS = ["morning", "afternoon", "evening", "night"]
ALL_DAY_BUCKET = "all_day"
WEEKDAY_TYPES = ["weekday", "weekend"]
ALL_WEEKDAY_TYPE = "all"

# The 9 scored contexts.
CONTEXTS = [(b, w) for w in WEEKDAY_TYPES for b in TIME_BUCKETS] + [
    (ALL_DAY_BUCKET, ALL_WEEKDAY_TYPE)
]

# Buckets are cyclically adjacent: morning-afternoon-evening-night-morning.
BUCKET_ADJACENCY = {
    "morning": {"afternoon", "night"},
    "afternoon": {"morning", "evening"},
    "evening": {"afternoon", "night"},
    "night": {"evening", "morning"},
}


def hour_to_time_bucket(hour: int) -> str:
    if 6 <= hour <= 11:
        return "morning"
    if 12 <= hour <= 17:
        return "afternoon"
    if 18 <= hour <= 21:
        return "evening"
    return "night"


def date_to_weekday_type(date: datetime.date) -> str:
    return "weekend" if date.weekday() >= 5 else "weekday"


# SQL twins of the helpers above, used by the set-based scoring queries.
SQL_TIME_BUCKET = """
CASE
    WHEN c.hour BETWEEN 6 AND 11 THEN 'morning'
    WHEN c.hour BETWEEN 12 AND 17 THEN 'afternoon'
    WHEN c.hour BETWEEN 18 AND 21 THEN 'evening'
    ELSE 'night'
END
"""

SQL_WEEKDAY_TYPE = """
CASE WHEN EXTRACT(ISODOW FROM c.date) >= 6 THEN 'weekend' ELSE 'weekday' END
"""

# Symmetric adjacent (crime_bucket, target_bucket) pairs as a SQL IN-list.
SQL_ADJACENT_PAIRS = """
(('morning','afternoon'),('afternoon','morning'),
 ('afternoon','evening'),('evening','afternoon'),
 ('evening','night'),('night','evening'),
 ('night','morning'),('morning','night'))
"""

# VALUES rows for the 9 contexts.
SQL_CONTEXTS_VALUES = ", ".join(f"('{b}','{w}')" for b, w in CONTEXTS)


def severity_case_sql(severity_weights: dict) -> str:
    """CASE expression mapping upper(crime_type) to its severity weight.

    Values come from risk_model_versions.parameters (trusted, operator-seeded);
    they are coerced to float and keys are sanity-checked, so the generated SQL
    carries no user input.
    """
    default = float(severity_weights.get("DEFAULT", 1.0))
    branches = []
    for crime_type, weight in sorted(severity_weights.items()):
        if crime_type == "DEFAULT":
            continue
        if not crime_type.replace(" ", "").replace("_", "").isalpha():
            raise ValueError(f"suspicious crime type key in parameters: {crime_type!r}")
        branches.append(f"WHEN '{crime_type.upper()}' THEN {float(weight)!r}")
    return "CASE upper(c.crime_type) " + " ".join(branches) + f" ELSE {default!r} END"


def self_test() -> None:
    assert hour_to_time_bucket(5) == "night"
    assert hour_to_time_bucket(6) == "morning"
    assert hour_to_time_bucket(11) == "morning"
    assert hour_to_time_bucket(12) == "afternoon"
    assert hour_to_time_bucket(17) == "afternoon"
    assert hour_to_time_bucket(18) == "evening"
    assert hour_to_time_bucket(21) == "evening"
    assert hour_to_time_bucket(22) == "night"
    assert hour_to_time_bucket(0) == "night"

    assert date_to_weekday_type(datetime.date(2026, 6, 12)) == "weekday"  # Friday
    assert date_to_weekday_type(datetime.date(2026, 6, 13)) == "weekend"  # Saturday
    assert date_to_weekday_type(datetime.date(2026, 6, 14)) == "weekend"  # Sunday

    for bucket, neighbors in BUCKET_ADJACENCY.items():
        for n in neighbors:
            assert bucket in BUCKET_ADJACENCY[n], f"adjacency not symmetric: {bucket}/{n}"

    assert len(CONTEXTS) == 9
    case = severity_case_sql({"ROBO": 1.5, "DEFAULT": 1.0})
    assert "WHEN 'ROBO' THEN 1.5" in case
    print("utils self-test OK")
