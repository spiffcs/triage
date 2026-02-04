package model

// IsTeamMember checks if a user is a collaborator based on authorAssociation
func IsTeamMember(association string) bool {
	switch association {
	case "MEMBER", "OWNER", "COLLABORATOR":
		return true
	default:
		// CONTRIBUTOR, FIRST_TIMER, FIRST_TIME_CONTRIBUTOR, NONE = external
		return false
	}
}
