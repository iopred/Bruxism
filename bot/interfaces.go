package bot

type Message interface {
  Channel() string
  UserName() string
  UserId() string
  Message() string
  MessageId() string
  IsModerator() bool
}

type Service interface {
  Name() string
  UserName() string
  Open() (<-chan Message, error)
  IsMe(message Message) bool
  SendMessage(channel, message string) error
  DeleteMessage(messageId string) error
  BanUser(channel, user string, duration int) error
  SetPlaying(game string) error
  Join(join string) error
}

type Plugin interface {
  Name() string
  Help() string
  Register(*Bot, Service, []byte) error
  Save() []byte
}