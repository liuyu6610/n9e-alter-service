package state

import "strings"

const (
	ChannelWebhook      = "webhook"
	ChannelRobotDefault = "robot:_default"
)

// ChannelRobot returns the per-destination key for a DingTalk robot.
// Unnamed/global robots share ChannelRobotDefault so they still get a stable slot.
func ChannelRobot(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ChannelRobotDefault
	}
	return "robot:" + id
}

// ChannelNotifiedAt is the last successful send time for channel.
// Records persisted before per-channel accounting have a nil map: LastNotified
// is treated as applying to every channel so an upgrade does not re-notify.
func (r Record) ChannelNotifiedAt(channel string) int64 {
	if len(r.ChannelNotified) == 0 {
		return r.LastNotified
	}
	return r.ChannelNotified[strings.TrimSpace(channel)]
}

// ChannelDue reports whether channel should be sent for this record.
// Recovered events notify once per channel; active events honor repeatSeconds.
func (r Record) ChannelDue(channel string, now int64, repeatSeconds int, recovered bool) bool {
	ts := r.ChannelNotifiedAt(channel)
	if recovered {
		return ts == 0
	}
	if repeatSeconds <= 0 {
		repeatSeconds = 3600
	}
	if ts > 0 && now-ts < int64(repeatSeconds) {
		return false
	}
	return true
}

// AnyChannelDue is true when at least one destination still needs a send.
func (r Record) AnyChannelDue(channels []string, now int64, repeatSeconds int, recovered bool) bool {
	if len(channels) == 0 {
		return false
	}
	for _, ch := range channels {
		if r.ChannelDue(ch, now, repeatSeconds, recovered) {
			return true
		}
	}
	return false
}
