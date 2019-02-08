package utils

// the stdlib provide a Math.Min method only for float64
func Min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
