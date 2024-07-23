package clusters

const (
	ReadWriteEnabled      = false
	ReadOnlyEnabled       = true
	StateNotInitialized   = false
	StateInitialized      = true
	ignoreIdentityChecker = "dynamodbbasedignoreidentitylistchecker"
	drStateChecker        = "dynamodbbasedstatechecker"
	AdmiralLeaseTableName = "admiral-lease"
)
