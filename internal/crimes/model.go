package crimes

type Crime struct {
	SourceID       string  `json:"source_id" bson:"source_id"`
	Year           int     `json:"year" bson:"year"`
	Month          int     `json:"month" bson:"month"`
	Day            int     `json:"day" bson:"day"`
	Date           string  `json:"date" bson:"date"`
	Hour           *int    `json:"hour" bson:"hour"`
	CrimeType      string  `json:"crime_type" bson:"crime_type"`
	CrimeSubtype   *string `json:"crime_subtype" bson:"crime_subtype"`
	WeaponUsed     *bool   `json:"weapon_used" bson:"weapon_used"`
	MotorcycleUsed *bool   `json:"motorcycle_used" bson:"motorcycle_used"`
	Neighborhood   *string `json:"neighborhood" bson:"neighborhood"`
	Commune        *int    `json:"commune" bson:"commune"`
	Quantity       int     `json:"quantity" bson:"quantity"`
	Location       GeoJSON `json:"location" bson:"location"`
}

type GeoJSON struct {
	Type        string    `json:"type" bson:"type"`
	Coordinates []float64 `json:"coordinates" bson:"coordinates"`
}
