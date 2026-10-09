package config

// BuiltinIndex returns the draft index for a builtin ID, or -1.
func (d MCPDraft) BuiltinIndex(id MCPBuiltinID) int {
	for i, item := range d.Builtins {
		if item.ID == id {
			return i
		}
	}
	return -1
}

// EnableBuiltin enables a builtin by ID.
func (d *MCPDraft) EnableBuiltin(id MCPBuiltinID) bool {
	i := d.BuiltinIndex(id)
	if i < 0 {
		return false
	}
	if !d.Builtins[i].Enabled {
		return d.ToggleBuiltin(i)
	}
	return true
}
