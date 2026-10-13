// Package memoryhealth provides read-only Memory diagnostics for Status/Doctor.
//
// It never creates Memory databases, directories, migrations, repairs, vacuum,
// reindex, or observation writes. Evidence is gathered via memory.OpenReadOnly
// and filesystem permission inspection only.
package memoryhealth
