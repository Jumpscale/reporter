package app

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/Jumpscale/reporter"
	"github.com/gin-gonic/gin"
)

func jsonAction(action func(ctx *gin.Context) (interface{}, error)) func(ctx *gin.Context) {
	return func(ctx *gin.Context) {
		obj, err := action(ctx)

		ctx.Header("content-type", "application/json")

		enc := json.NewEncoder(ctx.Writer)
		if err != nil {
			ctx.Writer.WriteHeader(http.StatusInternalServerError)
			if eerr := enc.Encode(err); eerr != nil {
				log.Errorf("failed to encode error (%s): %s", err, eerr)
			}
			return
		}

		ctx.Writer.WriteHeader(http.StatusOK)
		if err := enc.Encode(obj); err != nil {
			log.Errorf("failed to encode object (%v): %s", obj, err)
		}
	}
}

type API struct {
	InfluxRecorder  *reporter.InfluxRecorder
	AddressRecorder *reporter.AddressRecorder
}

func (a *API) Run(listen string) error {
	engine := gin.Default()

	engine.GET("height", jsonAction(a.height))
	engine.GET("tokens/total", jsonAction(a.total))
	engine.GET("tokens/transacted", jsonAction(a.transacted))
	engine.GET("address", jsonAction(a.addresses))
	engine.GET("address/:address", jsonAction(a.address))

	return engine.Run(listen)
}

func (a *API) height(ctx *gin.Context) (interface{}, error) {
	return a.InfluxRecorder.Height()
}

func (a *API) total(ctx *gin.Context) (interface{}, error) {
	return a.InfluxRecorder.TotalTokens()
}

func (a *API) transacted(ctx *gin.Context) (interface{}, error) {
	period := ctx.DefaultQuery("period", "1h")
	//TODO: validate given period
	return a.InfluxRecorder.TransactedToken(reporter.Period(period))
}

func (a *API) addresses(ctx *gin.Context) (interface{}, error) {
	var over float64
	var size int64
	var page int64

	var err error
	if over, err = strconv.ParseFloat(ctx.DefaultQuery("over", "0"), 64); err != nil {
		return nil, err
	}

	if size, err = strconv.ParseInt(ctx.DefaultQuery("size", "20"), 10, 32); err != nil {
		return nil, err
	}

	if page, err = strconv.ParseInt(ctx.DefaultQuery("page", "0"), 10, 32); err != nil {
		return nil, err
	}

	return a.AddressRecorder.Addresses(over, int(page), int(size))
}

func (a *API) address(ctx *gin.Context) (interface{}, error) {
	return a.AddressRecorder.Get(ctx.Param("address"))
}
