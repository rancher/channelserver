package scarf

type Gateway interface {
	RecordEvent(channel, resolved, latest, clusterID, clientIP string)
}
