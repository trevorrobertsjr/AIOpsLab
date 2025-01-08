package frontend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"google.golang.org/grpc"

	recommendation "github.com/harlow/go-micro-services/services/recommendation/proto"
	reservation "github.com/harlow/go-micro-services/services/reservation/proto"
	user "github.com/harlow/go-micro-services/services/user/proto"
	"github.com/rs/zerolog/log"

	"github.com/harlow/go-micro-services/dialer"
	"github.com/harlow/go-micro-services/registry"
	profile "github.com/harlow/go-micro-services/services/profile/proto"
	search "github.com/harlow/go-micro-services/services/search/proto"
	"github.com/harlow/go-micro-services/tls"
	httptrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http"
)

// Server implements frontend service
type Server struct {
	searchClient         search.SearchClient
	profileClient        profile.ProfileClient
	recommendationClient recommendation.RecommendationClient
	userClient           user.UserClient
	reservationClient    reservation.ReservationClient
	KnativeDns           string
	IpAddr               string
	Port                 int
	// Tracer               tracer.Tracer
	Registry *registry.Client
}

// Run the server
func (s *Server) Run() error {
	if s.Port == 0 {
		return fmt.Errorf("Server port must be set")
	}

	log.Info().Msg("Initializing gRPC clients...")
	if err := s.initSearchClient("srv-search"); err != nil {
		return err
	}

	if err := s.initProfileClient("srv-profile"); err != nil {
		return err
	}

	if err := s.initRecommendationClient("srv-recommendation"); err != nil {
		return err
	}

	if err := s.initUserClient("srv-user"); err != nil {
		return err
	}

	if err := s.initReservation("srv-reservation"); err != nil {
		return err
	}
	log.Info().Msg("Successfull")

	log.Trace().Msg("frontend before mux")
	mux := httptrace.NewServeMux()
	mux.Handle("/", http.FileServer(http.Dir("services/frontend/static")))
	mux.Handle("/hotels", httptrace.WrapHandler(http.HandlerFunc(s.searchHandler), "frontend", "/hotels"))
	mux.Handle("/recommendations", httptrace.WrapHandler(http.HandlerFunc(s.recommendHandler), "frontend", "/recommendations"))
	mux.Handle("/user", httptrace.WrapHandler(http.HandlerFunc(s.userHandler), "frontend", "/user"))
	mux.Handle("/reservation", httptrace.WrapHandler(http.HandlerFunc(s.reservationHandler), "frontend", "/reservation"))

	log.Trace().Msg("frontend starts serving")

	tlsconfig := tls.GetHttpsOpt()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port),
		Handler: mux,
	}
	if tlsconfig != nil {
		log.Info().Msg("Serving https")
		srv.TLSConfig = tlsconfig
		return srv.ListenAndServeTLS("x509/server_cert.pem", "x509/server_key.pem")
	} else {
		log.Info().Msg("Serving http")
		return srv.ListenAndServe()
	}
}

func (s *Server) initSearchClient(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("dialer error: %v", err)
	}
	s.searchClient = search.NewSearchClient(conn)
	return nil
}

func (s *Server) initProfileClient(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("dialer error: %v", err)
	}
	s.profileClient = profile.NewProfileClient(conn)
	return nil
}

func (s *Server) initRecommendationClient(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("dialer error: %v", err)
	}
	s.recommendationClient = recommendation.NewRecommendationClient(conn)
	return nil
}

func (s *Server) initUserClient(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("dialer error: %v", err)
	}
	s.userClient = user.NewUserClient(conn)
	return nil
}

func (s *Server) initReservation(name string) error {
	conn, err := s.getGrpcConn(name)
	if err != nil {
		return fmt.Errorf("dialer error: %v", err)
	}
	s.reservationClient = reservation.NewReservationClient(conn)
	return nil
}

func (s *Server) getGrpcConn(name string) (*grpc.ClientConn, error) {
	// If Knative DNS is configured, use it for the service address
	if s.KnativeDns != "" {
		target := fmt.Sprintf("%s.%s", name, s.KnativeDns)
		log.Info().Msgf("Dialing Knative DNS target: %s", target)
		return dialer.Dial(target, dialer.WithTracer())
	}

	// Default to Consul-based service discovery
	target := fmt.Sprintf("consul:///%s", name)
	log.Info().Msgf("Dialing Consul target: %s at %s for %s", target, s.Registry.Address(), name)
	return dialer.Dial(target, dialer.WithTracer())
}

// DEBUG
func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()

	log.Trace().Msg("searchHandler invoked")

	// in/out dates from query params
	inDate, outDate := r.URL.Query().Get("inDate"), r.URL.Query().Get("outDate")
	if inDate == "" || outDate == "" {
		log.Debug().Msg("inDate or outDate parameter is missing")
		http.Error(w, "Please specify inDate/outDate params", http.StatusBadRequest)
		return
	}

	// lat/lon from query params
	sLat, sLon := r.URL.Query().Get("lat"), r.URL.Query().Get("lon")
	if sLat == "" || sLon == "" {
		log.Debug().Msg("lat or lon parameter is missing")
		http.Error(w, "Please specify location params", http.StatusBadRequest)
		return
	}

	Lat, _ := strconv.ParseFloat(sLat, 32)
	lat := float32(Lat)
	Lon, _ := strconv.ParseFloat(sLon, 32)
	lon := float32(Lon)

	log.Debug().Msgf("Parsed request parameters: lat=%v, lon=%v, inDate=%s, outDate=%s", lat, lon, inDate, outDate)

	// search for best hotels
	searchResp, err := s.searchClient.Nearby(ctx, &search.NearbyRequest{
		Lat:     lat,
		Lon:     lon,
		InDate:  inDate,
		OutDate: outDate,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error fetching search results")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msgf("Received search response with hotel IDs: %v", searchResp.HotelIds)

	// grab locale from query params or default to en
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = "en"
	}

	// check availability
	reservationResp, err := s.reservationClient.CheckAvailability(ctx, &reservation.Request{
		HotelId:    searchResp.HotelIds,
		InDate:     inDate,
		OutDate:    outDate,
		RoomNumber: 1,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error checking availability")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msgf("Availability response received for hotel IDs: %v", reservationResp.HotelId)

	// fetch hotel profiles
	profileResp, err := s.profileClient.GetProfiles(ctx, &profile.Request{
		HotelIds: reservationResp.HotelId,
		Locale:   locale,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error fetching hotel profiles")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msgf("Profile response received with hotels: %+v", profileResp.Hotels)

	json.NewEncoder(w).Encode(geoJSONResponse(profileResp.Hotels))
	log.Trace().Msg("searchHandler completed successfully")
}

func (s *Server) recommendHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()

	log.Trace().Msg("recommendHandler invoked")

	// Parse latitude and longitude
	sLat, sLon := r.URL.Query().Get("lat"), r.URL.Query().Get("lon")
	if sLat == "" || sLon == "" {
		log.Debug().Msg("lat or lon parameter is missing")
		http.Error(w, "Please specify location params", http.StatusBadRequest)
		return
	}
	Lat, _ := strconv.ParseFloat(sLat, 64)
	lat := float64(Lat)
	Lon, _ := strconv.ParseFloat(sLon, 64)
	lon := float64(Lon)

	// Parse 'require' parameter
	require := r.URL.Query().Get("require")
	if require != "dis" && require != "rate" && require != "price" {
		log.Debug().Msg("Invalid or missing 'require' parameter")
		http.Error(w, "Please specify require params", http.StatusBadRequest)
		return
	}

	log.Debug().Msgf("Parsed request parameters: lat=%v, lon=%v, require=%s", lat, lon, require)

	// fetch recommendations
	recResp, err := s.recommendationClient.GetRecommendations(ctx, &recommendation.Request{
		Require: require,
		Lat:     lat,
		Lon:     lon,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error fetching recommendations")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msgf("Received recommendations with hotel IDs: %v", recResp.HotelIds)

	// grab locale from query params or default to en
	locale := r.URL.Query().Get("locale")
	if locale == "" {
		locale = "en"
	}

	// fetch hotel profiles
	profileResp, err := s.profileClient.GetProfiles(ctx, &profile.Request{
		HotelIds: recResp.HotelIds,
		Locale:   locale,
	})
	if err != nil {
		log.Error().Err(err).Msg("Error fetching hotel profiles")
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Debug().Msgf("Profile response received with hotels: %+v", profileResp.Hotels)

	json.NewEncoder(w).Encode(geoJSONResponse(profileResp.Hotels))
	log.Trace().Msg("recommendHandler completed successfully")
}

// DEBUG

// func (s *Server) searchHandler(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Access-Control-Allow-Origin", "*")
// 	ctx := r.Context()

// 	// Start a new Datadog span
// 	span, ctx := tracer.StartSpanFromContext(ctx, "frontend.searchHandler", tracer.ServiceName("frontend"))
// 	defer span.Finish()

// 	log.Trace().Msg("starts searchHandler")

// 	// in/out dates from query params
// 	inDate, outDate := r.URL.Query().Get("inDate"), r.URL.Query().Get("outDate")
// 	if inDate == "" || outDate == "" {
// 		http.Error(w, "Please specify inDate/outDate params", http.StatusBadRequest)
// 		span.SetTag(ext.Error, true)
// 		return
// 	}

// 	// lan/lon from query params
// 	sLat, sLon := r.URL.Query().Get("lat"), r.URL.Query().Get("lon")
// 	if sLat == "" || sLon == "" {
// 		http.Error(w, "Please specify location params", http.StatusBadRequest)
// 		return
// 	}

// 	Lat, _ := strconv.ParseFloat(sLat, 32)
// 	lat := float32(Lat)
// 	Lon, _ := strconv.ParseFloat(sLon, 32)
// 	lon := float32(Lon)

// 	log.Trace().Msg("starts searchHandler querying downstream")

// 	log.Trace().Msgf("SEARCH [lat: %v, lon: %v, inDate: %v, outDate: %v", lat, lon, inDate, outDate)
// 	// search for best hotels
// 	searchResp, err := s.searchClient.Nearby(ctx, &search.NearbyRequest{
// 		Lat:     lat,
// 		Lon:     lon,
// 		InDate:  inDate,
// 		OutDate: outDate,
// 	})
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	log.Trace().Msg("SearchHandler gets searchResp")
// 	//for _, hid := range searchResp.HotelIds {
// 	//	log.Trace().Msgf("Search Handler hotelId = %s", hid)
// 	//}

// 	// grab locale from query params or default to en
// 	locale := r.URL.Query().Get("locale")
// 	if locale == "" {
// 		locale = "en"
// 	}

// 	reservationResp, err := s.reservationClient.CheckAvailability(ctx, &reservation.Request{
// 		CustomerName: "",
// 		HotelId:      searchResp.HotelIds,
// 		InDate:       inDate,
// 		OutDate:      outDate,
// 		RoomNumber:   1,
// 	})
// 	if err != nil {
// 		log.Error().Msg("SearchHandler CheckAvailability failed")
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	log.Trace().Msgf("searchHandler gets reserveResp")
// 	log.Trace().Msgf("searchHandler gets reserveResp.HotelId = %s", reservationResp.HotelId)

// 	// hotel profiles
// 	profileResp, err := s.profileClient.GetProfiles(ctx, &profile.Request{
// 		HotelIds: reservationResp.HotelId,
// 		Locale:   locale,
// 	})
// 	if err != nil {
// 		log.Error().Msg("SearchHandler GetProfiles failed")
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	log.Trace().Msg("searchHandler gets profileResp")

// 	json.NewEncoder(w).Encode(geoJSONResponse(profileResp.Hotels))
// }

// func (s *Server) recommendHandler(w http.ResponseWriter, r *http.Request) {
// 	w.Header().Set("Access-Control-Allow-Origin", "*")
// 	ctx := r.Context()

// 	sLat, sLon := r.URL.Query().Get("lat"), r.URL.Query().Get("lon")
// 	if sLat == "" || sLon == "" {
// 		http.Error(w, "Please specify location params", http.StatusBadRequest)
// 		return
// 	}
// 	Lat, _ := strconv.ParseFloat(sLat, 64)
// 	lat := float64(Lat)
// 	Lon, _ := strconv.ParseFloat(sLon, 64)
// 	lon := float64(Lon)

// 	require := r.URL.Query().Get("require")
// 	if require != "dis" && require != "rate" && require != "price" {
// 		http.Error(w, "Please specify require params", http.StatusBadRequest)
// 		return
// 	}

// 	// recommend hotels
// 	recResp, err := s.recommendationClient.GetRecommendations(ctx, &recommendation.Request{
// 		Require: require,
// 		Lat:     float64(lat),
// 		Lon:     float64(lon),
// 	})
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	// grab locale from query params or default to en
// 	locale := r.URL.Query().Get("locale")
// 	if locale == "" {
// 		locale = "en"
// 	}

// 	// hotel profiles
// 	profileResp, err := s.profileClient.GetProfiles(ctx, &profile.Request{
// 		HotelIds: recResp.HotelIds,
// 		Locale:   locale,
// 	})
// 	if err != nil {
// 		http.Error(w, err.Error(), http.StatusInternalServerError)
// 		return
// 	}

// 	json.NewEncoder(w).Encode(geoJSONResponse(profileResp.Hotels))
// }

func (s *Server) userHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()

	username, password := r.URL.Query().Get("username"), r.URL.Query().Get("password")
	if username == "" || password == "" {
		http.Error(w, "Please specify username and password", http.StatusBadRequest)
		return
	}

	// Check username and password
	recResp, err := s.userClient.CheckUser(ctx, &user.Request{
		Username: username,
		Password: password,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	str := "Login successfully!"
	if recResp.Correct == false {
		str = "Failed. Please check your username and password. "
	}

	res := map[string]interface{}{
		"message": str,
	}

	json.NewEncoder(w).Encode(res)
}

func (s *Server) reservationHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ctx := r.Context()

	inDate, outDate := r.URL.Query().Get("inDate"), r.URL.Query().Get("outDate")
	if inDate == "" || outDate == "" {
		http.Error(w, "Please specify inDate/outDate params", http.StatusBadRequest)
		return
	}

	if !checkDataFormat(inDate) || !checkDataFormat(outDate) {
		http.Error(w, "Please check inDate/outDate format (YYYY-MM-DD)", http.StatusBadRequest)
		return
	}

	hotelId := r.URL.Query().Get("hotelId")
	if hotelId == "" {
		http.Error(w, "Please specify hotelId params", http.StatusBadRequest)
		return
	}

	customerName := r.URL.Query().Get("customerName")
	if customerName == "" {
		http.Error(w, "Please specify customerName params", http.StatusBadRequest)
		return
	}

	username, password := r.URL.Query().Get("username"), r.URL.Query().Get("password")
	if username == "" || password == "" {
		http.Error(w, "Please specify username and password", http.StatusBadRequest)
		return
	}

	numberOfRoom := 0
	num := r.URL.Query().Get("number")
	if num != "" {
		numberOfRoom, _ = strconv.Atoi(num)
	}

	// Check username and password
	recResp, err := s.userClient.CheckUser(ctx, &user.Request{
		Username: username,
		Password: password,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	str := "Reserve successfully!"
	if recResp.Correct == false {
		str = "Failed. Please check your username and password. "
	}

	// Make reservation
	resResp, err := s.reservationClient.MakeReservation(ctx, &reservation.Request{
		CustomerName: customerName,
		HotelId:      []string{hotelId},
		InDate:       inDate,
		OutDate:      outDate,
		RoomNumber:   int32(numberOfRoom),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(resResp.HotelId) == 0 {
		str = "Failed. Already reserved. "
	}

	res := map[string]interface{}{
		"message": str,
	}

	json.NewEncoder(w).Encode(res)
}

// return a geoJSON response that allows google map to plot points directly on map
// https://developers.google.com/maps/documentation/javascript/datalayer#sample_geojson
func geoJSONResponse(hs []*profile.Hotel) map[string]interface{} {
	fs := []interface{}{}

	for _, h := range hs {
		fs = append(fs, map[string]interface{}{
			"type": "Feature",
			"id":   h.Id,
			"properties": map[string]string{
				"name":         h.Name,
				"phone_number": h.PhoneNumber,
			},
			"geometry": map[string]interface{}{
				"type": "Point",
				"coordinates": []float32{
					h.Address.Lon,
					h.Address.Lat,
				},
			},
		})
	}

	return map[string]interface{}{
		"type":     "FeatureCollection",
		"features": fs,
	}
}

func checkDataFormat(date string) bool {
	if len(date) != 10 {
		return false
	}
	for i := 0; i < 10; i++ {
		if i == 4 || i == 7 {
			if date[i] != '-' {
				return false
			}
		} else {
			if date[i] < '0' || date[i] > '9' {
				return false
			}
		}
	}
	return true
}
