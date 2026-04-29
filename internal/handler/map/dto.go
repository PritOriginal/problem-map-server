package maprest

import "github.com/PritOriginal/problem-map-server/internal/models"

type GetAdminBoundariesResponse struct {
	AdminBoundaries []models.AdminBoundary `json:"admin_boundaries"`
}

type GetAdminBoundariesMarksCountResponse struct {
	AdminBoundaries []models.AdminBoundaryMarksCount `json:"admin_boundaries"`
}

type GetRegionsResponse struct {
	Regions []models.Region `json:"regions"`
}

type GetCitiesResponse struct {
	Cities []models.City `json:"cities"`
}

type GetDistrictsResponse struct {
	Districts []models.District `json:"districts"`
}
