package beads

// BeadSummary represents a bead in list output
type BeadSummary struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Labels []string `json:"labels"`
}
