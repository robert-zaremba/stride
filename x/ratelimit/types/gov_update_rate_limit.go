package types

import (
	"fmt"

	"regexp"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
)

const (
	ProposalTypeUpdateRateLimit = "UpdateRateLimit"
)

func init() {
	govtypes.RegisterProposalType(ProposalTypeUpdateRateLimit)
	govtypes.RegisterProposalTypeCodec(&UpdateRateLimitProposal{}, "stride.ratelimit.UpdateRateLimitProposal")
}

var (
	_ govtypes.Content = &UpdateRateLimitProposal{}
)

func NewUpdateRateLimitProposal(title, description, denom, channelId string, maxPercentSend sdk.Int, maxPercentRecv sdk.Int, durationHours uint64) govtypes.Content {
	return &UpdateRateLimitProposal{
		Title:          title,
		Description:    description,
		Denom:          denom,
		ChannelId:      channelId,
		MaxPercentSend: maxPercentSend,
		MaxPercentRecv: maxPercentRecv,
		DurationHours:  durationHours,
	}
}

func (p *UpdateRateLimitProposal) GetTitle() string { return p.Title }

func (p *UpdateRateLimitProposal) GetDescription() string { return p.Description }

func (p *UpdateRateLimitProposal) ProposalRoute() string { return RouterKey }

func (p *UpdateRateLimitProposal) ProposalType() string {
	return ProposalTypeUpdateRateLimit
}

func (p *UpdateRateLimitProposal) ValidateBasic() error {
	err := govtypes.ValidateAbstract(p)
	if err != nil {
		return err
	}

	if p.Denom == "" {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "invalid denom (%s)", p.Denom)
	}

	matched, err := regexp.MatchString(`^channel-\d+$`, p.ChannelId)
	if err != nil {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "unable to verify channel-id (%s)", p.ChannelId)
	}
	if !matched {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "invalid channel-id (%s), must be of the format 'channel-{N}'", p.ChannelId)
	}

	if p.MaxPercentSend.GT(sdk.NewInt(100)) || p.MaxPercentSend.LT(sdk.ZeroInt()) {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "max-percent-send percent must be between 0 and 100 (inclusively), Provided: %v", p.MaxPercentSend)
	}

	if p.MaxPercentRecv.GT(sdk.NewInt(100)) || p.MaxPercentRecv.LT(sdk.ZeroInt()) {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "max-percent-recv percent must be between 0 and 100 (inclusively), Provided: %v", p.MaxPercentRecv)
	}

	if p.MaxPercentRecv.IsZero() && p.MaxPercentSend.IsZero() {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "either the max send or max receive threshold must be greater than 0")
	}

	if p.DurationHours == 0 {
		return sdkerrors.Wrapf(sdkerrors.ErrInvalidRequest, "duration can not be zero")
	}

	return nil
}

func (p UpdateRateLimitProposal) String() string {
	return fmt.Sprintf(`Update Rate Limit Proposal:
	Title:           %s
	Description:     %s
	Denom:           %s
	ChannelId:      %s
	MaxPercentSend: %v
	MaxPercentRecv: %v
	DurationHours:  %d
  `, p.Title, p.Description, p.Denom, p.ChannelId, p.MaxPercentSend, p.MaxPercentRecv, p.DurationHours)
}