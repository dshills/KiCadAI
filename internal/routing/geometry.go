package routing

type Point struct {
	XMM float64 `json:"x_mm"`
	YMM float64 `json:"y_mm"`
}

type Size struct {
	WidthMM  float64 `json:"width_mm"`
	HeightMM float64 `json:"height_mm"`
}

type Rect struct {
	Min Point `json:"min"`
	Max Point `json:"max"`
}

type Shape struct {
	Rect *Rect `json:"rect,omitempty"`
}
