package authrest

type SignUpRequest struct {
	Username string `json:"username" binding:"required,min=2,max=40"`
	Login    string `json:"login" binding:"required,min=3,max=40"`
	Password string `json:"password" binding:"required,min=8,max=64"`
}

type SignUpResponse struct {
	UserId int `json:"user_id"`
}

type SignInRequest struct {
	Login    string `json:"login" binding:"required,min=3,max=40"`
	Password string `json:"password" binding:"required,min=8,max=64"`
}

type SignInResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type RefreshTokensRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required,jwt"`
}

type RefreshTokensResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}
