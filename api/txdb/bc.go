package txdb

import (
	"database/sql"

	"golang.org/x/net/context"

	"chain/api/utxodb"
	"chain/database/pg"
	"chain/errors"
	"chain/fedchain/bc"
	"chain/fedchain/state"
)

type Output struct {
	state.Output
	ManagerNodeID string
	AccountID     string
	AddrIndex     [2]uint32
}

func loadOutput(ctx context.Context, p bc.Outpoint) (*state.Output, error) {
	const q = `
		SELECT asset_id, amount, script, metadata
		FROM utxos
		WHERE txid=$1 AND index=$2
	`
	o := &state.Output{
		Outpoint: p,

		// If the utxo row exists, it is considered unspent. This function does
		// not (and should not) consider spending activity in the tx pool, which
		// is handled by poolView.
		Spent: false,
	}
	err := pg.FromContext(ctx).QueryRow(q, p.Hash.String(), p.Index).Scan(
		&o.AssetID,
		&o.Value,
		&o.Script,
		&o.Metadata,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	} else if err != nil {
		return nil, errors.Wrap(err)
	}
	return o, nil
}

// LoadUTXOs loads all unspent outputs in the blockchain
// for the given asset and account.
func LoadUTXOs(ctx context.Context, accountID, assetID string) ([]*utxodb.UTXO, error) {
	// TODO(kr): account stuff will split into a separate
	// table and this will become something like
	// LoadUTXOs(context.Context, []bc.Outpoint) []*bc.TxOutput.

	const q = `
		SELECT amount, reserved_until, txid, index
		FROM utxos
		WHERE account_id=$1 AND asset_id=$2
	`
	rows, err := pg.FromContext(ctx).Query(q, accountID, assetID)
	if err != nil {
		return nil, errors.Wrap(err, "query")
	}
	defer rows.Close()
	var utxos []*utxodb.UTXO
	for rows.Next() {
		u := &utxodb.UTXO{
			AccountID: accountID,
			AssetID:   assetID,
		}
		var txid string
		err = rows.Scan(
			&u.Amount,
			&u.ResvExpires,
			&txid,
			&u.Outpoint.Index,
		)
		if err != nil {
			return nil, errors.Wrap(err, "scan")
		}
		h, err := bc.ParseHash(txid)
		if err != nil {
			return nil, errors.Wrap(err, "decode hash")
		}
		u.Outpoint.Hash = h
		u.ResvExpires = u.ResvExpires.UTC()
		utxos = append(utxos, u)
	}
	return utxos, errors.Wrap(rows.Err(), "rows")
}
