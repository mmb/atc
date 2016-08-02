module Login exposing (..)

import Html exposing (Html)
import Html.Attributes as Attributes
import Html.Events as Events
import Http
import Task

import Concourse.Team exposing (Team)

type alias Model =
  { teamName : Maybe String
  , teamFilter : String
  , teams : Maybe (List Team)
  }

type Action
  = Noop
  | FilterTeams String
  | TeamsFetched (Result Http.Error (List Team))

type alias Flags =
  { teamName : String
  , redirect : String
  }

init : Flags -> (Model, Cmd Action)
init flags =
  let
    model =
      { teamName =
          case flags.teamName of
            "" -> Nothing
            _ -> Just flags.teamName
      , teamFilter = ""
      , teams = Nothing
      }
  in
    ( model
    , Cmd.map TeamsFetched <| Task.perform Err Ok Concourse.Team.fetchTeams
    )

update : Action -> Model -> (Model, Cmd Action)
update action model =
  case action of
    Noop ->
      (model, Cmd.none)
    FilterTeams newTeamFilter ->
      ( { model | teamFilter = newTeamFilter }
      , Cmd.none
      )
    TeamsFetched (Ok teams) ->
      ( { model | teams = Just teams }
      , Cmd.none
      )
    TeamsFetched (Err err) ->
      Debug.log ("failed to fetch teams: " ++ toString err) <|
        (model, Cmd.none)

view : Model -> Html Action
view model = Html.div []
  [ Html.div []
      [ Html.text <|
          case model.teamName of
            Nothing -> "who am i"
            Just teamName -> "hello world " ++ teamName
      ]
  , Html.div []
      [ Html.input
          [ Attributes.placeholder "filter teams"
          , Events.onInput FilterTeams
          ]
          []
      ]
  , Html.div []
      [ Html.text <| "you searched for " ++ model.teamFilter
      ]
  , Html.div [] <| List.map viewTeam <| Maybe.withDefault [] model.teams
  ]

viewTeam : Team -> Html action
viewTeam team = Html.div [] [ Html.text <| "team: " ++ team.name ]
