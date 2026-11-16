package wire

// Command identifies a message on the NESYS pipe. Values below 0x100 are sent by the game,
// values from 0x101 are sent by the service.
type Command uint32

// Commands sent by the game to the service.
const (
	ClientStart       Command = 0x01
	ConnectRequest    Command = 0x02
	DisconnectRequest Command = 0x03
	GameStart         Command = 0x04
	GameEnd           Command = 0x05
	GameContinue      Command = 0x06
	EventDownload     Command = 0x07
	EventRequest      Command = 0x08
	CardSelect        Command = 0x09
	CardInsert        Command = 0x0A
	CardUpdate        Command = 0x0B
	CardBuysItem      Command = 0x0C
	CardTakeover      Command = 0x0D
	CardForceTakeover Command = 0x0E
	CardDecrease      Command = 0x0F
	CardReissueTest   Command = 0x10
	CardReissue       Command = 0x11
	CardPlayedList    Command = 0x12
	RankingData       Command = 0x13
	LocalNetworkInfo  Command = 0x14
	GlobalAddress     Command = 0x15
	Echo              Command = 0x16
	AdapterInfo       Command = 0x17
	ServiceVersion    Command = 0x18
	DhcpRenew         Command = 0x19
	HTTPGet           Command = 0x1A
	HTTPPost          Command = 0x1B
	UploadConfig      Command = 0x1C
	IncomeStart       Command = 0x1D
	IncomeEnd         Command = 0x1E
	IncomeContinue    Command = 0x1F
	SetIncomeMode     Command = 0x20
	DestroyMyService  Command = 0x21
	IncomePoint       Command = 0x22
	GameStatusReset   Command = 0x23
	RowEventDataList  Command = 0x24
	Shopping          Command = 0x25
	HTTPSGet          Command = 0x26
	HTTPSPost         Command = 0x27
	FreeTicket        Command = 0x28
	GameFreeStart     Command = 0x29
	GameFreeEnd       Command = 0x2A
	IncomeFreeStart   Command = 0x2B
	IncomeFreeEnd     Command = 0x2C
	ClientEnd         Command = 0xFF
	ClientError       Command = 0xFFFFFFFF
)

// Commands sent by the service to the game.
const (
	NetworkError         Command = 0x101
	CertError            Command = 0x102
	NetworkRecoverNotice Command = 0x103
	SoonMaintenance      Command = 0x104
	LinkupNotice         Command = 0x105
	LinkLocalModeNotice  Command = 0x106
	CertInitNotice       Command = 0x107
	CertRegularNotice    Command = 0x108
	EffectiveEvent       Command = 0x109
	IneffectiveEvent     Command = 0x10A
	DhcpRenewStart       Command = 0x10B
	DhcpCompleteNotice   Command = 0x10C
	ClientStartReply     Command = 0x10D
	ConnectReply         Command = 0x10E
	DisconnectReply      Command = 0x10F
	GameStatusReply      Command = 0x110
	CardSelectReply      Command = 0x111
	CardInsertReply      Command = 0x112
	CardUpdateReply      Command = 0x113
	CardBuysItemReply    Command = 0x114
	CardTakeoverReply    Command = 0x115
	CardDecreaseReply    Command = 0x116
	CardReissueTestReply Command = 0x117
	CardReissueReply     Command = 0x118
	CardPlayedListReply  Command = 0x119
	RankingDataReply     Command = 0x11A
	LocalNetworkReply    Command = 0x11B
	LocalNetworkNotice   Command = 0x11C
	GlobalAddressReply   Command = 0x11D
	EchoReply            Command = 0x11E
	AdapterInfoReply     Command = 0x11F
	ServiceVersionReply  Command = 0x120
	HTTPStart            Command = 0x121
	HTTPReply            Command = 0x122
	UploadConfigReply    Command = 0x123
	IncomeStatusReply    Command = 0x124
	SetIncomeModeReply   Command = 0x125
	DestroyMyServiceNote Command = 0x126
	GameStatusResetReply Command = 0x127
	RowEventDataReply    Command = 0x128
	ShoppingReply        Command = 0x129
	FreeTicketReply      Command = 0x12A
	ClientEndReply       Command = 0x1FF
)
