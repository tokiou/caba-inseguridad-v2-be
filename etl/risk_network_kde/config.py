"""Default model parameters for network_temporal_edge_risk_v1.

The database row (risk_model_versions.parameters) is the source of truth for a
given model version; these defaults exist to seed new candidate versions during
calibration. Severity weights cover the dataset's real crime taxonomy.
"""

DEFAULT_MODEL_PARAMETERS = {
    "crime_snap_max_distance_meters": 80,
    "snap_distance_decay_meters": 30,

    "network_bandwidth_meters": 350,
    "network_distance_decay_meters": 100,

    "recency_half_life_days": 365,

    "severity_base_weights": {
        "HOMICIDIOS": 3.0,
        "ROBO": 1.5,
        "LESIONES": 1.2,
        "HURTO": 1.0,
        "AMENAZAS": 0.7,
        "VIALIDAD": 0.3,
        "DEFAULT": 1.0,
    },

    "weapon_multiplier": 1.5,
    "motorcycle_multiplier": 1.25,

    "temporal_weights": {
        "same_bucket": 1.0,
        "adjacent_bucket": 0.5,
        "other_bucket": 0.2,
    },

    "weekday_weights": {
        "same_type": 1.0,
        "other_type": 0.25,
        "all": 1.0,
    },

    "normalization": {
        "method": "p95_clamp",
    },

    "risk_levels": {
        "low_max": 0.33,
        "moderate_max": 0.66,
        "high_max": 1.0,
    },
}

# Calibration grid (section 10 of the incoming spec). Finite by construction.
CALIBRATION_GRID = {
    "network_bandwidth_meters": [250, 350, 500],
    "network_distance_decay_meters": [75, 100, 150],
    "recency_half_life_days": [180, 365, 730],
    "weapon_multiplier": [1.3, 1.5, 1.8],
    "motorcycle_multiplier": [1.15, 1.25, 1.4],
}

EVALUATION_GATE = {
    "pai_top5_min": 3.0,
    "recall_top10_min": 0.30,
    "top_decile_lift_min": 3.0,
}

# Weighted selection score for calibration candidates.
SELECTION_WEIGHTS = {
    "pai_top5": 0.45,
    "recall_top10": 0.35,
    "top_decile_lift": 0.20,
}
