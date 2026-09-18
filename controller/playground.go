package controller

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"

	"github.com/gin-gonic/gin"
)

func Playground(c *gin.Context) {
	playgroundRelay(c, types.RelayFormatOpenAI)
}

// PlaygroundImage relays text-to-image and image-edit requests from the
// playground workbench.
func PlaygroundImage(c *gin.Context) {
	playgroundRelay(c, types.RelayFormatOpenAIImage)
}

// PlaygroundAudio relays speech synthesis and transcription requests from the
// playground workbench.
func PlaygroundAudio(c *gin.Context) {
	playgroundRelay(c, types.RelayFormatOpenAIAudio)
}

// playgroundRelay runs a relay on behalf of the signed-in user without an API
// key: the browser only holds a session cookie, so an ephemeral token is built
// per request and never persisted.
func playgroundRelay(c *gin.Context, relayFormat types.RelayFormat) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	if _, newAPIError = preparePlaygroundRequest(c, relayFormat); newAPIError != nil {
		return
	}

	Relay(c, relayFormat)
}

// PlaygroundVideo submits or fetches an asynchronous video task from the
// playground workbench. Video is a task endpoint rather than a streaming relay,
// so it goes through RelayTask/RelayTaskFetch instead of Relay.
func PlaygroundVideo(c *gin.Context) {
	playgroundTask(c, RelayTask)
}

// PlaygroundThreeD submits a 3D generation task from the playground workbench.
func PlaygroundThreeD(c *gin.Context) {
	playgroundTask(c, RelayTask)
}

// PlaygroundTaskFetch retrieves a task the playground submitted earlier. The
// task id comes from the URL, so the same handler serves video and 3D.
func PlaygroundTaskFetch(c *gin.Context) {
	playgroundTask(c, RelayTaskFetch)
}

// playgroundTask installs the same ephemeral-token context the streaming
// playground uses, then hands off to the task handler.
func playgroundTask(c *gin.Context, handler gin.HandlerFunc) {
	var newAPIError *types.NewAPIError

	defer func() {
		if newAPIError != nil {
			c.JSON(newAPIError.StatusCode, gin.H{
				"error": newAPIError.ToOpenAIError(),
			})
		}
	}()

	if _, newAPIError = preparePlaygroundRequest(c, types.RelayFormatTask); newAPIError != nil {
		return
	}

	handler(c)
}

// preparePlaygroundRequest builds the relay info for a playground request and
// installs the ephemeral token that stands in for a user API key.
func preparePlaygroundRequest(c *gin.Context, relayFormat types.RelayFormat) (*relaycommon.RelayInfo, *types.NewAPIError) {
	useAccessToken := c.GetBool("use_access_token")
	if useAccessToken {
		return nil, types.NewError(errors.New("暂不支持使用 access token"), types.ErrorCodeAccessDenied, types.ErrOptionWithSkipRetry())
	}

	relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, nil, nil)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	userId := c.GetInt("id")

	// Write user context to ensure acceptUnsetRatio is available
	userCache, err := model.GetUserCache(userId)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	userCache.WriteContext(c)

	tempToken := &model.Token{
		UserId: userId,
		Name:   fmt.Sprintf("playground-%s", relayInfo.UsingGroup),
		Group:  relayInfo.UsingGroup,
	}
	_ = middleware.SetupContextForToken(c, tempToken)

	return relayInfo, nil
}
