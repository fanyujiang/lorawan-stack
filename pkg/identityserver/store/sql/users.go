// Copyright © 2018 The Things Network Foundation, distributed under the MIT license (see LICENSE file)

package sql

import (
	"fmt"
	"strings"

	"github.com/TheThingsNetwork/ttn/pkg/errors"
	"github.com/TheThingsNetwork/ttn/pkg/identityserver/db"
	"github.com/TheThingsNetwork/ttn/pkg/identityserver/store"
	"github.com/TheThingsNetwork/ttn/pkg/identityserver/types"
	"github.com/TheThingsNetwork/ttn/pkg/ttnpb"
)

// UserStore implements store.UserStore.
type UserStore struct {
	storer
	*extraAttributesStore
}

func NewUserStore(store storer) *UserStore {
	return &UserStore{
		storer:               store,
		extraAttributesStore: newExtraAttributesStore(store, "user"),
	}
}

// Create creates an user.
func (s *UserStore) Create(user types.User) error {
	err := s.transact(func(tx *db.Tx) error {
		err := s.create(tx, user)
		if err != nil {
			return err
		}

		return s.storeAttributes(tx, user.GetUser().UserID, user, nil)
	})
	return err
}

func (s *UserStore) create(q db.QueryContext, user types.User) error {
	u := user.GetUser()
	_, err := q.NamedExec(
		`INSERT
			INTO users (
				user_id,
				name,
				email,
				password,
				validated_at)
			VALUES (
				lower(:user_id),
				:name,
				lower(:email),
				:password,
				:validated_at)`,
		u)

	if duplicates, yes := db.IsDuplicate(err); yes {
		if email, duplicated := duplicates["email"]; duplicated {
			return ErrUserEmailTaken.New(errors.Attributes{
				"email": email,
			})
		}
		if userID, duplicated := duplicates["user_id"]; duplicated {
			return ErrUserIDTaken.New(errors.Attributes{
				"user_id": userID,
			})
		}
	}

	return err
}

// GetByID finds the user by ID and returns it.
func (s *UserStore) GetByID(userID string, factory store.UserFactory) (types.User, error) {
	result := factory()

	err := s.transact(func(tx *db.Tx) error {
		err := s.getByID(tx, userID, result)
		if err != nil {
			return err
		}

		return s.loadAttributes(tx, result.GetUser().UserID, result)
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *UserStore) getByID(q db.QueryContext, userID string, result types.User) error {
	err := q.SelectOne(
		result,
		`SELECT *
			FROM users
			WHERE user_id = lower($1)`,
		userID)
	if db.IsNoRows(err) {
		return ErrUserNotFound.New(errors.Attributes{
			"user_id": userID,
		})
	}
	return err
}

// GetByEmail finds the user by email address and returns it.
func (s *UserStore) GetByEmail(email string, factory store.UserFactory) (types.User, error) {
	result := factory()

	err := s.transact(func(tx *db.Tx) error {
		err := s.getByEmail(tx, email, result)
		if err != nil {
			return err
		}

		return s.loadAttributes(tx, result.GetUser().UserID, result)
	})

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *UserStore) getByEmail(q db.QueryContext, email string, result types.User) error {
	err := q.SelectOne(
		result,
		`SELECT *
			FROM users
			WHERE email = lower($1)`,
		email)
	if db.IsNoRows(err) {
		return ErrUserEmailNotFound.New(errors.Attributes{
			"email": email,
		})
	}
	return err
}

// Update updates an user.
func (s *UserStore) Update(user types.User) error {
	err := s.transact(func(tx *db.Tx) error {
		err := s.update(tx, user)
		if err != nil {
			return err
		}

		return s.storeAttributes(s.queryer(), user.GetUser().UserID, user, nil)
	})
	return err
}

func (s *UserStore) update(q db.QueryContext, user types.User) error {
	u := user.GetUser()

	_, err := q.NamedExec(
		`UPDATE users
			SET name = :name,
				email = lower(:email),
				validated_at = :validated_at,
				password = :password,
				admin = :admin,
				updated_at = current_timestamp()
			WHERE user_id = :user_id`,
		u)

	if _, yes := db.IsDuplicate(err); yes {
		return ErrUserEmailTaken.New(errors.Attributes{
			"email": u.Email,
		})
	}

	return err
}

// SaveValidationToken saves the validation token.
func (s *UserStore) SaveValidationToken(userID string, token *types.ValidationToken) error {
	return s.saveValidationToken(s.queryer(), userID, token)
}

func (s *UserStore) saveValidationToken(q db.QueryContext, userID string, token *types.ValidationToken) error {
	_, err := q.Exec(
		`INSERT
			INTO validation_tokens (
				validation_token,
				user_id,
				created_at,
				expires_in
			)
			VALUES (
				$1,
				$2,
				$3,
				$4)`,
		token.ValidationToken,
		userID,
		token.CreatedAt,
		token.ExpiresIn)
	return err
}

// GetValidationToken retrieves the validation token.
func (s *UserStore) GetValidationToken(userID, token string) (*types.ValidationToken, error) {
	return s.getValidationToken(s.queryer(), userID, token)
}

func (s *UserStore) getValidationToken(q db.QueryContext, userID, token string) (*types.ValidationToken, error) {
	t := new(types.ValidationToken)
	err := q.SelectOne(
		t,
		`SELECT
				validation_token,
				created_at,
				expires_in
			FROM validation_tokens
			WHERE validation_token = $1 AND user_id = $2`,
		token,
		userID)
	if db.IsNoRows(err) {
		return nil, ErrValidationTokenNotFound.New(nil)
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteValidationToken deletes the validation token.
func (s *UserStore) DeleteValidationToken(userID, token string) error {
	return s.deleteValidationToken(s.queryer(), userID, token)
}

func (s *UserStore) deleteValidationToken(q db.QueryContext, userID, token string) error {
	t := new(string)
	err := q.SelectOne(
		t,
		`DELETE
			FROM validation_tokens
			WHERE validation_token = $1 AND user_id = $2
			RETURNING validation_token`,
		token,
		userID)
	if db.IsNoRows(err) {
		return ErrValidationTokenNotFound.New(nil)
	}
	return err
}

func (s *UserStore) SaveAPIKey(userID string, key *ttnpb.APIKey) error {
	err := s.transact(func(tx *db.Tx) error {
		err := s.saveAPIKey(tx, userID, key)
		if err != nil {
			return err
		}

		return s.saveAPIKeyRights(tx, userID, key)
	})
	return err
}

func (s *UserStore) saveAPIKey(q db.QueryContext, userID string, key *ttnpb.APIKey) error {
	_, err := q.Exec(
		`INSERT
			INTO users_api_keys (user_id, key, key_name)
			VALUES ($1, $2, $3)`,
		userID,
		key.Key,
		key.Name)
	if _, yes := db.IsDuplicate(err); yes {
		return ErrAPIKeyNameConflict.New(errors.Attributes{
			"name": key.Name,
		})
	}
	return err
}

func (s *UserStore) saveAPIKeyRights(q db.QueryContext, userID string, key *ttnpb.APIKey) error {
	query, args := s.saveAPIKeyRightsQuery(userID, key)
	_, err := q.Exec(query, args...)
	return err
}

func (s *UserStore) saveAPIKeyRightsQuery(userID string, key *ttnpb.APIKey) (string, []interface{}) {
	args := make([]interface{}, 1+len(key.Rights))
	args[0] = key.Key

	boundValues := make([]string, len(key.Rights))

	for i, right := range key.Rights {
		args[i+1] = right
		boundValues[i] = fmt.Sprintf("($1, $%d)", i+2)
	}

	query := fmt.Sprintf(
		`INSERT
			INTO users_api_keys_rights (key, "right")
			VALUES %s
			ON CONFLICT (key, "right")
			DO NOTHING`,
		strings.Join(boundValues, ", "))

	return query, args
}

func (s *UserStore) GetAPIKey(userID, keyName string) (*ttnpb.APIKey, error) {
	var key *ttnpb.APIKey
	var err error
	err = s.transact(func(tx *db.Tx) error {
		key, err = s.getAPIKey(tx, userID, keyName)
		if err != nil {
			return err
		}

		key.Rights, err = s.getAPIKeyRights(tx, key.Key)
		return err
	})
	if err != nil {
		return nil, err
	}
	return key, nil
}

func (s *UserStore) getAPIKey(q db.QueryContext, userID, keyName string) (*ttnpb.APIKey, error) {
	res := new(ttnpb.APIKey)
	err := q.SelectOne(
		res,
		`SELECT
				key,
				key_name AS name
			FROM users_api_keys
			WHERE user_id = $1 AND key_name = $2`,
		userID,
		keyName)
	if db.IsNoRows(err) {
		return nil, ErrAPIKeyNotFound.New(errors.Attributes{
			"name": keyName,
		})
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *UserStore) getAPIKeyRights(q db.QueryContext, key string) ([]ttnpb.Right, error) {
	var res []ttnpb.Right
	err := q.Select(
		&res,
		`SELECT
				"right"
			FROM users_api_keys_rights
			WHERE key = $1`,
		key)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *UserStore) ListAPIKeys(userID string) ([]*ttnpb.APIKey, error) {
	var res []*ttnpb.APIKey
	var err error
	err = s.transact(func(tx *db.Tx) error {
		res, err = s.listAPIKeys(tx, userID)
		if err != nil {
			return err
		}

		for i, key := range res {
			res[i].Rights, err = s.getAPIKeyRights(tx, key.Key)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *UserStore) listAPIKeys(q db.QueryContext, userID string) ([]*ttnpb.APIKey, error) {
	var res []*ttnpb.APIKey
	err := q.Select(
		&res,
		`SELECT
				key,
				key_name AS name
			FROM users_api_keys
			WHERE user_id = $1`,
		userID)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s *UserStore) UpdateAPIKey(userID string, key *ttnpb.APIKey) error {
	err := s.transact(func(tx *db.Tx) error {
		err := s.deleteAPIKeyRights(tx, key.Key)
		if err != nil {
			return err
		}

		return s.saveAPIKeyRights(tx, userID, key)
	})
	return err
}

func (s *UserStore) DeleteAPIKey(userID, keyName string) error {
	err := s.transact(func(tx *db.Tx) error {
		key, err := s.getAPIKey(tx, userID, keyName)
		if err != nil {
			return err
		}

		err = s.deleteAPIKeyRights(tx, key.Key)
		if err != nil {
			return err
		}

		return s.deleteAPIKey(tx, userID, keyName)
	})
	return err
}

func (s *UserStore) deleteAPIKey(q db.QueryContext, userID, keyName string) error {
	res := new(string)
	err := q.SelectOne(
		res,
		`DELETE
			FROM users_api_keys
			WHERE user_id = $1 AND key_name = $2
			RETURNING key`,
		userID,
		keyName)
	if db.IsNoRows(err) {
		return ErrAPIKeyNotFound.New(errors.Attributes{
			"name": keyName,
		})
	}
	return err
}

func (s *UserStore) deleteAPIKeyRights(q db.QueryContext, key string) error {
	_, err := q.Exec(`DELETE FROM users_api_keys_rights WHERE key = $1`, key)
	return err
}

// LoadAttributes loads the extra attributes in user if it is a store.Attributer.
func (s *UserStore) LoadAttributes(userID string, user types.User) error {
	return s.loadAttributes(s.queryer(), userID, user)
}

func (s *UserStore) loadAttributes(q db.QueryContext, userID string, user types.User) error {
	attr, ok := user.(store.Attributer)
	if ok {
		return s.extraAttributesStore.loadAttributes(q, userID, attr)
	}

	return nil
}

// StoreAttributes store the extra attributes of user if it is a store.Attributer
// and writes the resulting user in result.
func (s *UserStore) StoreAttributes(userID string, user, result types.User) error {
	return s.storeAttributes(s.queryer(), userID, user, result)
}

func (s *UserStore) storeAttributes(q db.QueryContext, userID string, user, result types.User) error {
	attr, ok := user.(store.Attributer)
	if ok {
		res, ok := result.(store.Attributer)
		if result == nil || !ok {
			return s.extraAttributesStore.storeAttributes(q, userID, attr, nil)
		}

		return s.extraAttributesStore.storeAttributes(q, userID, attr, res)
	}

	return nil
}
