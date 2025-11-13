package tributary

// PostgreSQL NOTIFY channel names used for inter-process communication.
const (
	// ChannelStepAvailable is the channel used to notify when new steps become available.
	ChannelStepAvailable = "tributary_step_available"

	// ChannelStepCompleted is the channel used to notify when steps are completed.
	ChannelStepCompleted = "tributary_step_completed"
)
