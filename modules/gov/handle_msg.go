package gov

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cosmos/cosmos-sdk/x/authz"

	"github.com/forbole/bdjuno/v4/types"

	govtypes "github.com/atomone-hub/govgen/x/gov/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	juno "github.com/forbole/juno/v5/types"
)

// HandleMsgExec implements modules.AuthzMessageModule
func (m *Module) HandleMsgExec(index int, _ *authz.MsgExec, _ int, executedMsg sdk.Msg, tx *juno.Tx) error {
	return m.HandleMsg(index, executedMsg, tx)
}

// HandleMsg implements modules.MessageModule
func (m *Module) HandleMsg(index int, msg sdk.Msg, tx *juno.Tx) error {
	if len(tx.Logs) == 0 {
		return nil
	}

	switch cosmosMsg := msg.(type) {
	case *govtypes.MsgSubmitProposal:
		return m.handleMsgSubmitProposal(tx, index, cosmosMsg)
	case *govtypes.MsgDeposit:
		return m.handleMsgDeposit(tx, cosmosMsg)
	case *govtypes.MsgVote:
		return m.handleMsgVote(tx, cosmosMsg)
	case *govtypes.MsgVoteWeighted:
		return m.handleMsgVoteWeighted(tx, cosmosMsg)
	}

	return nil
}

// handleMsgSubmitProposal allows to properly handle MsgSubmitProposal
func (m *Module) handleMsgSubmitProposal(tx *juno.Tx, index int, msg *govtypes.MsgSubmitProposal) error {
	// Get the proposal id
	event, err := tx.FindEventByType(index, govtypes.EventTypeSubmitProposal)
	if err != nil {
		return fmt.Errorf("error while searching for EventTypeSubmitProposal: %s", err)
	}

	id, err := tx.FindAttributeByKey(event, govtypes.AttributeKeyProposalID)
	if err != nil {
		return fmt.Errorf("error while searching for AttributeKeyProposalID: %s", err)
	}

	proposalID, err := strconv.ParseUint(id, 10, 64)
	if err != nil {
		return fmt.Errorf("error while parsing proposal id: %s", err)
	}

	// Get the proposal
	proposal, err := m.source.Proposal(tx.Height, proposalID)
	if err != nil {
		return fmt.Errorf("error while getting proposal: %s", err)
	}

	// Store the proposal
	proposalObj := types.NewProposal(
		proposal.ProposalId,
		proposal.ProposalRoute(),
		proposal.ProposalType(),
		msg.GetContent(),
		proposal.Status.String(),
		proposal.SubmitTime,
		proposal.DepositEndTime,
		proposal.VotingStartTime,
		proposal.VotingEndTime,
		msg.Proposer,
	)
	err = m.db.SaveProposals([]types.Proposal{proposalObj})
	if err != nil {
		return err
	}

	txTimestamp, err := time.Parse(time.RFC3339, tx.Timestamp)
	if err != nil {
		return fmt.Errorf("error while parsing time: %s", err)
	}

	// Store the deposit
	deposit := types.NewDeposit(proposal.ProposalId, msg.Proposer, msg.InitialDeposit, txTimestamp, tx.Height)
	return m.db.SaveDeposits([]types.Deposit{deposit})
}

// handleMsgDeposit allows to properly handle a MsgDeposit
func (m *Module) handleMsgDeposit(tx *juno.Tx, msg *govtypes.MsgDeposit) error {
	deposit, err := m.source.ProposalDeposit(tx.Height, msg.ProposalId, msg.Depositor)
	if err != nil {
		return fmt.Errorf("error while getting proposal deposit: %s", err)
	}

	txTimestamp, err := time.Parse(time.RFC3339, tx.Timestamp)
	if err != nil {
		return fmt.Errorf("error while parsing time: %s", err)
	}

	return m.db.SaveDeposits([]types.Deposit{
		types.NewDeposit(msg.ProposalId, msg.Depositor, deposit.Amount, txTimestamp, tx.Height),
	})
}

// handleMsgVote allows to properly handle a MsgVote
func (m *Module) handleMsgVote(tx *juno.Tx, msg *govtypes.MsgVote) error {
	txTimestamp, err := time.Parse(time.RFC3339, tx.Timestamp)
	if err != nil {
		return fmt.Errorf("error while parsing time: %s", err)
	}

	vote := types.NewVote(msg.ProposalId, msg.Voter, msg.Option, "1.0", txTimestamp, tx.Height)
	err = m.db.CleanVote(msg.ProposalId, msg.Voter)
	if err != nil {
		return err
	}
	return m.db.SaveVote(vote)
}

func (m *Module) handleMsgVoteWeighted(tx *juno.Tx, msg *govtypes.MsgVoteWeighted) error {
	txTimestamp, err := time.Parse(time.RFC3339, tx.Timestamp)
	if err != nil {
		return fmt.Errorf("error while parsing time: %s", err)
	}
	err = m.db.CleanVote(msg.ProposalId, msg.Voter)
	if err != nil {
		return err
	}
	for _, option := range msg.Options {
		vote := types.NewVote(msg.ProposalId, msg.Voter, option.Option, option.Weight.String(), txTimestamp, tx.Height)
		err = m.db.SaveVote(vote)
		if err != nil {
			return fmt.Errorf("error while saving weighted vote for address %s: %s", msg.Voter, err)
		}
	}

	// update tally result for given proposal
	return m.updateProposalTallyResult(msg.ProposalId, tx.Height)
}
