package permissions

type Level int

const (
	LevelUser Level = iota
	LevelDJ
	LevelAdmin
)

func (l Level) String() string {
	switch l {
	case LevelUser:
		return "User"
	case LevelDJ:
		return "DJ"
	case LevelAdmin:
		return "Admin"
	default:
		return "Unknown"
	}
}

type Config struct {
	DJRoleName    string
	AdminRoleName string
}
