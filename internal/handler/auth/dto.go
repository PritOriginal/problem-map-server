package authrest

type SignUpRequest struct {
	Username string `json:"username" validate:"required,min=2,max=40"`
	Login    string `json:"login" validate:"required,min=3,max=40,alphanum"`
	Password string `json:"password" validate:"required,min=8,max=64"`
}

type SignUpResponse struct {
	UserId string `json:"user_id"`
}

type SignInRequest struct {
	Login    string `json:"login" validate:"required,min=3,max=40,alphanum"`
	Password string `json:"password" validate:"required,min=8,max=64"`
}

type SignInResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type RefreshTokensRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required,jwt"`
}

type RefreshTokensResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
