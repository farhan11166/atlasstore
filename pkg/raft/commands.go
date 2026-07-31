package raft

// this folder is for the log command we only have two types of commans nodes join and node leave.
import "encoding/json"

type CommandType string

const (
	CmdNodeJoin  CommandType = "node_join"
	CmdNodeLeave CommandType = "node_leave"
)

type Command struct {
	Type    CommandType `json:"type"`
	Address string      `json:"address"`
}

func (c Command) Encode() ([]byte, error) {
	return json.Marshal((c))
}

func DecodeCommand(data []byte) (Command, error) {
	var cmd Command
	err := json.Unmarshal(data, &cmd)
	return cmd, err
}
