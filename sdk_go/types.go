package allyourbase

import "encoding/json"

type User struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	IsAnonymous   *bool   `json:"isAnonymous,omitempty"`
	LinkedAt      *string `json:"linkedAt,omitempty"`
	EmailVerified *bool   `json:"emailVerified,omitempty"`
	CreatedAt     string  `json:"createdAt,omitempty"`
	UpdatedAt     *string `json:"updatedAt,omitempty"`
}

// UnmarshalJSON unmarshals JSON data into u, supporting both camelCase and snake_case naming conventions with camelCase taking precedence.
func (u *User) UnmarshalJSON(data []byte) error {
	type userWire struct {
		ID                 string  `json:"id"`
		Email              string  `json:"email"`
		IsAnonymous        *bool   `json:"isAnonymous"`
		IsAnonymousSnake   *bool   `json:"is_anonymous"`
		LinkedAt           *string `json:"linkedAt"`
		LinkedAtSnake      *string `json:"linked_at"`
		EmailVerified      *bool   `json:"emailVerified"`
		EmailVerifiedSnake *bool   `json:"email_verified"`
		CreatedAt          string  `json:"createdAt"`
		CreatedAtSnake     string  `json:"created_at"`
		UpdatedAt          *string `json:"updatedAt"`
		UpdatedAtSnake     *string `json:"updated_at"`
	}

	var wire userWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	u.ID = wire.ID
	u.Email = wire.Email

	if wire.IsAnonymous != nil {
		u.IsAnonymous = wire.IsAnonymous
	} else {
		u.IsAnonymous = wire.IsAnonymousSnake
	}

	if wire.LinkedAt != nil {
		u.LinkedAt = wire.LinkedAt
	} else {
		u.LinkedAt = wire.LinkedAtSnake
	}

	if wire.EmailVerified != nil {
		u.EmailVerified = wire.EmailVerified
	} else {
		u.EmailVerified = wire.EmailVerifiedSnake
	}

	if wire.CreatedAt != "" {
		u.CreatedAt = wire.CreatedAt
	} else {
		u.CreatedAt = wire.CreatedAtSnake
	}

	if wire.UpdatedAt != nil {
		u.UpdatedAt = wire.UpdatedAt
	} else {
		u.UpdatedAt = wire.UpdatedAtSnake
	}

	return nil
}

type AuthResponse struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	User         User   `json:"user"`
}

type MagicLinkRequestResponse struct {
	Message string `json:"message"`
}

type MagicLinkConfirmResponse struct {
	Auth       *AuthResponse `json:"-"`
	MFAToken   string        `json:"-"`
	MFAPending bool          `json:"-"`
}

func (r *MagicLinkConfirmResponse) UnmarshalJSON(data []byte) error {
	type confirmWire struct {
		MFAPending      bool            `json:"mfa_pending"`
		MFAPendingCamel bool            `json:"mfaPending"`
		MFAToken        string          `json:"mfa_token"`
		MFATokenCamel   string          `json:"mfaToken"`
		Token           string          `json:"token"`
		RefreshToken    string          `json:"refreshToken"`
		User            json.RawMessage `json:"user"`
	}

	var wire confirmWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}

	r.MFAPending = wire.MFAPending || wire.MFAPendingCamel
	if wire.MFAToken != "" {
		r.MFAToken = wire.MFAToken
	} else {
		r.MFAToken = wire.MFATokenCamel
	}
	if r.MFAPending {
		return nil
	}

	var auth AuthResponse
	if err := json.Unmarshal(data, &auth); err != nil {
		return err
	}
	r.Auth = &auth
	return nil
}

type ListResponse struct {
	Items      []map[string]any `json:"items"`
	Page       int              `json:"page"`
	PerPage    int              `json:"perPage"`
	TotalItems int              `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

type BatchOperation struct {
	Method string         `json:"method"`
	ID     string         `json:"id,omitempty"`
	Body   map[string]any `json:"body,omitempty"`
}

type BatchResult struct {
	Index  int            `json:"index"`
	Status int            `json:"status"`
	Body   map[string]any `json:"body,omitempty"`
}

type StorageObject struct {
	ID          string  `json:"id"`
	Bucket      string  `json:"bucket"`
	Name        string  `json:"name"`
	Size        int64   `json:"size"`
	ContentType string  `json:"contentType"`
	UserID      *string `json:"userId,omitempty"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   *string `json:"updatedAt,omitempty"`
}

type StorageListResponse struct {
	Items      []StorageObject `json:"items"`
	TotalItems int             `json:"totalItems"`
}
