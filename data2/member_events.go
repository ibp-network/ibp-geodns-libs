package data2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ibp-network/ibp-geodns-libs/matrix"
)

// -----------------------------------------------------------------------------
// TYPES
// -----------------------------------------------------------------------------

type NetStatusRecord struct {
	CheckType int
	CheckName string
	CheckURL  string
	Domain    string
	Member    string
	Status    bool
	IsIPv6    bool
	StartTime time.Time
	EndTime   sql.NullTime
	Error     string
	VoteData  map[string]bool
	Extra     map[string]interface{}
}

// -----------------------------------------------------------------------------
// HELPERS
// -----------------------------------------------------------------------------

func ctToString(ct int) string {
	switch ct {
	case 1:
		return "site"
	case 2:
		return "domain"
	case 3:
		return "endpoint"
	default:
		return "unknown"
	}
}

func boolToTiny(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullOrString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func shouldNotifyOffline(status bool, rowsAffected int64) bool {
	return !status && rowsAffected == 1
}

// -----------------------------------------------------------------------------
// DB OPERATIONS + MATRIX NOTIFICATIONS
// -----------------------------------------------------------------------------

func InsertNetStatus(rec NetStatusRecord) error {
	jVotes, err := json.Marshal(rec.VoteData)
	if err != nil {
		return fmt.Errorf("marshal vote data: %w", err)
	}
	jExtra, err := json.Marshal(rec.Extra)
	if err != nil {
		return fmt.Errorf("marshal extra data: %w", err)
	}

	ctString := ctToString(rec.CheckType)
	if ctString == "unknown" {
		return fmt.Errorf("unsupported check type %d", rec.CheckType)
	}

	// Ensure StartTime is UTC
	if rec.StartTime.Location() != time.UTC {
		rec.StartTime = rec.StartTime.UTC()
	}

	q := `INSERT INTO member_events
		(check_type,check_name,endpoint,domain_name,member_name,status,is_ipv6,start_time,error,vote_data,additional_data)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE
		  status      = VALUES(status),
		  vote_data   = VALUES(vote_data),
		  end_time    = IF(VALUES(status)=1,UTC_TIMESTAMP(),NULL)`

	result, err := DB.Exec(q,
		ctString,
		rec.CheckName,
		rec.CheckURL,
		rec.Domain,
		rec.Member,
		boolToTiny(rec.Status),
		boolToTiny(rec.IsIPv6),
		rec.StartTime,
		nullOrString(rec.Error),
		string(jVotes),
		string(jExtra),
	)

	if err == nil {
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if shouldNotifyOffline(rec.Status, affected) {
			// New outage ⇒ alert
			matrix.NotifyMemberOffline(
				rec.Member,
				ctToString(rec.CheckType),
				rec.CheckName,
				rec.Domain,
				rec.CheckURL,
				rec.IsIPv6,
				rec.Error,
			)
		}
	}

	return err
}

func CloseOpenEvent(rec NetStatusRecord) error {
	ctString := ctToString(rec.CheckType)
	if ctString == "unknown" {
		return fmt.Errorf("unsupported check type %d", rec.CheckType)
	}

	q := `UPDATE member_events
		SET end_time = UTC_TIMESTAMP(), status = 1
		WHERE check_type=? AND check_name=? AND endpoint=? AND domain_name=? AND member_name=? AND is_ipv6=? AND status=0 AND end_time IS NULL`

	result, err := DB.Exec(q,
		ctString,
		rec.CheckName,
		rec.CheckURL,
		rec.Domain,
		rec.Member,
		boolToTiny(rec.IsIPv6),
	)

	if err == nil {
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if affected == 0 {
			return nil
		}

		// Outage resolved ⇒ notify
		matrix.NotifyMemberOnline(
			rec.Member,
			ctToString(rec.CheckType),
			rec.CheckName,
			rec.Domain,
			rec.CheckURL,
			rec.IsIPv6,
		)
	}

	return err
}
