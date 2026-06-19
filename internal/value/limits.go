package value

// Thresholds bound a compact collection. A collection promotes to its full
// representation the moment it would exceed MaxEntries elements, or stores an
// element larger than MaxBytes. Promotion is one-way; a collection never
// demotes, which keeps the logic simple and avoids thrashing at the boundary.
type Thresholds struct {
	MaxEntries int
	MaxBytes   int
}

// exceeded reports whether a collection now holding count elements, the largest
// of the just-added ones being elemSize bytes, must leave the compact encoding.
func (t Thresholds) exceeded(count, elemSize int) bool {
	return count > t.MaxEntries || elemSize > t.MaxBytes
}

// Limits bundles the compact-encoding thresholds for each collection kind.
type Limits struct {
	List Thresholds
	Hash Thresholds
	Set  Thresholds
	ZSet Thresholds
}
