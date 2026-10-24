package exporter

// Shared Prometheus label names (and the occasional label value) used by
// metric descriptors, query handlers and log fields. Defining them once
// keeps descriptors and their usage sites consistent.
const (
	labelAddress          = "address"
	labelMoniker          = "moniker"
	labelDenom            = "denom"
	labelStatus           = "status"
	labelValidator        = "validator"
	labelValidatorAddress = "validator_address"
	labelRedelegatedTo    = "redelegated_to"
	labelEstimatedTime    = "estimated_time"
	labelHeight           = "height"
	labelTitle            = "title"
	labelVoteOption       = "vote_option"
	labelVoted            = "voted"
	labelVotingEndTime    = "voting_end_time"
	labelInfo             = "info"
	labelName             = "name"
	labelVotingStartTime  = "voting_start_time"
	labelValueNone        = "None"
)
