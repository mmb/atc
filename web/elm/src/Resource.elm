module Resource exposing (Flags, init, update, view)

import Concourse
import Html exposing (Html)
import Html.Attributes exposing (class, href, id, disabled, attribute)

type alias Model =
  { resourceIdentifier : Concourse.ResourceIdentifier
  }


type Msg
  = Noop

type alias Flags =
  { teamName : String
  , pipelineName : String
  , resourceName : String
  , pageSince : Int
  , pageUntil : Int
  }


init : Flags -> (Model, Cmd Msg)
init flags =
  let
    model =
      { resourceIdentifier =
          { teamName = flags.teamName
          , pipelineName = flags.pipelineName
          , resourceName = flags.resourceName
          }
      }
  in
    ( model
    , Cmd.batch
      []
    )

update : Msg -> Model -> (Model, Cmd Msg)
update action model =
  (model,Cmd.batch [])

view : Model -> Html Msg
view model =
  Html.div [class "with-fixed-header"] []
