package router

import "elitegate/internal/model"

var routes = []model.Route{
	{
		Path:        "/api/users",
		UpstreamURL: "http://localhost:9001",
	},
	{
		Path:        "/api/orders",
		UpstreamURL: "http://localhost:9002",
	},
}

func Match(path string)*model.Route{
	for _,route:= range routes{
       if route.Path==path{
		return &route

	   }
	}
	return nil
}