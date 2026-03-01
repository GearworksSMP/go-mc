package block

func IsAir(s StateID) bool {
	if int(s) >= len(StateList) || StateList[s] == nil {
		return false
	}
	return IsAirBlock(StateList[s])
}

func IsAirBlock(b Block) bool {
	switch b.(type) {
	case Air, CaveAir, VoidAir:
		return true
	default:
		return false
	}
}
