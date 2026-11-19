package domain

// SubscriptionBatch is the progress of one queued batch ingestion: every
// selected movie subscription either records a magnet, stays waiting for a
// qualifying magnet, or failed.
type SubscriptionBatch struct {
	Total     int                   `json:"total"`
	Processed int                   `json:"processed"`
	Submitted int                   `json:"submitted"`
	Waiting   int                   `json:"waiting"`
	Failed    int                   `json:"failed"`
	Failures  []SubscriptionFailure `json:"failures,omitempty"`
}

// SubscriptionFailure names one movie that could not be submitted.
type SubscriptionFailure struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}
