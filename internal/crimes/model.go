package crimes

type Crime struct {
	SourceID       string  `json:"source_id"`
	Year           int     `json:"year"`
	Month          int     `json:"month"`
	Day            int     `json:"day"`
	Date           string  `json:"date"`
	Hour           *int    `json:"hour"`
	CrimeType      string  `json:"crime_type"`
	CrimeSubtype   *string `json:"crime_subtype"`
	WeaponUsed     *bool   `json:"weapon_used"`
	MotorcycleUsed *bool   `json:"motorcycle_used"`
	Neighborhood   *string `json:"neighborhood"`
	Commune        *int    `json:"commune"`
	Quantity       int     `json:"quantity"`
	Location       GeoJSON `json:"location"`
}

type GeoJSON struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"`
}
