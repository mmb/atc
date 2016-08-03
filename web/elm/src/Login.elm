module Login exposing (..)

import Html exposing (Html)
import Html.Attributes as Attributes exposing (id, class)
import Html.Events as Events
import Http
import Navigation
import String
import Task

import Concourse.AuthMethod exposing (AuthMethod (..))
import Concourse.Team exposing (Team)

type Page = TeamSelectionPage | LoginPage String

type Model
  = TeamSelection TeamSelectionModel
  | Login LoginModel

type alias TeamSelectionModel =
  { teamFilter : String
  , teams : Maybe (List Team)
  }

type alias LoginModel =
  { teamName : String
  , authMethods : Maybe (List AuthMethod)
  }

type Action
  = Noop
  | FilterTeams String
  | TeamsFetched (Result Http.Error (List Team))
  | SelectTeam String
  | AuthFetched (Result Http.Error (List AuthMethod))

type alias Flags =
  { redirect : String
  }

init : Flags -> Result String Page -> (Model, Cmd Action)
init flags pageResult =
  case Result.withDefault TeamSelectionPage pageResult of
    TeamSelectionPage ->
      ( TeamSelection
          { teamFilter = ""
          , teams = Nothing
          }
      , Cmd.map TeamsFetched <| Task.perform Err Ok Concourse.Team.fetchTeams
      )
    LoginPage teamName ->
      ( Login
          { teamName = teamName
          , authMethods = Nothing
          }
      , Cmd.map
          AuthFetched <|
          Task.perform
            Err Ok <|
              Concourse.AuthMethod.fetchAuthMethods teamName
      )

urlUpdate : Result String Page -> Model -> (Model, Cmd Action)
urlUpdate pageResult model =
  case Result.withDefault TeamSelectionPage pageResult of
    TeamSelectionPage ->
      ( TeamSelection
          { teamFilter = ""
          , teams = Nothing
          }
      , Cmd.map TeamsFetched <| Task.perform Err Ok Concourse.Team.fetchTeams
      )
    LoginPage teamName ->
      ( Login
          { teamName = teamName
          , authMethods = Nothing
          }
      , Cmd.map
          AuthFetched <|
          Task.perform
            Err Ok <|
              Concourse.AuthMethod.fetchAuthMethods teamName
      )

update : Action -> Model -> (Model, Cmd Action)
update action model =
  case action of
    Noop ->
      (model, Cmd.none)
    FilterTeams newTeamFilter ->
      case model of
        TeamSelection teamSelectionModel ->
          ( TeamSelection { teamSelectionModel | teamFilter = newTeamFilter }
          , Cmd.none
          )
        Login _ -> (model, Cmd.none)
    TeamsFetched (Ok teams) ->
      case model of
        TeamSelection teamSelectionModel ->
          ( TeamSelection { teamSelectionModel | teams = Just teams }
          , Cmd.none
          )
        Login _ -> (model, Cmd.none)
    TeamsFetched (Err err) ->
      Debug.log ("failed to fetch teams: " ++ toString err) <|
        (model, Cmd.none)
    SelectTeam teamName ->
      (model, Navigation.newUrl <| "teams/" ++ teamName ++ "/login")
    AuthFetched (Ok authMethods) ->
      case model of
        Login loginModel ->
          ( Login { loginModel | authMethods = Just authMethods }
          , Cmd.none
          )
        TeamSelection tModel ->
          (model, Cmd.none)
    AuthFetched (Err err) ->
      Debug.log ("failed to fetch auth methods: " ++ toString err) <|
        (model, Cmd.none)

view : Model -> Html Action
view model =
  case model of
    TeamSelection tModel ->
      let filteredTeams =
        filterTeams tModel.teamFilter <| Maybe.withDefault [] tModel.teams
      in
        Html.div
          [ class "centered-contents" ]
          [ Html.div
              [ class "small-title" ]
              [ Html.text "select a team to login" ]
          , Html.div
              [ class "login-box" ] <|
              [ Html.form
                  [ Events.onSubmit <|
                      case (List.head filteredTeams, tModel.teamFilter) of
                        (Nothing, _) -> Noop
                        (Just _, "") -> Noop
                        (Just firstTeam, _) -> SelectTeam firstTeam.name
                  ]
                  [ Html.i [class "fa fa-fw fa-search"] []
                  , Html.input
                      [ Attributes.placeholder "filter teams"
                      , Events.onInput FilterTeams
                      ]
                      []
                  ]
              ] ++
                List.map viewTeam filteredTeams
          ]

    Login lModel ->
      Html.div
        [] <|
        [ Html.div [] [ Html.text <| "logging in to " ++ lModel.teamName ]
        ] ++
          ( if List.member BasicMethod <| Maybe.withDefault [] lModel.authMethods then
              [ Html.form
                [ Attributes.method "post" ]
                [ Html.div []
                    [ Html.label
                        [ Attributes.for "basic-auth-username-input" ]
                        [ Html.text "username" ]
                    ]
                , Html.div []
                    [ Html.input
                        [ id "basic-auth-username-input"
                        , Attributes.name "username"
                        , Attributes.type' "text"
                        ]
                        []
                    ]
                , Html.div []
                    [ Html.label
                        [ Attributes.for "basic-auth-password-input" ]
                        [ Html.text "password" ]
                    ]
                , Html.div []
                    [ Html.input
                        [ id "basic-auth-password-input"
                        , Attributes.name "password"
                        , Attributes.type' "password"
                        ]
                        []
                    ]
                , Html.div []
                    [ Html.button
                        [ Attributes.type' "submit" ]
                        [ Html.text "login" ]
                    ]
                ]
              ]
            else
              []
          ) ++
          ( List.filterMap
              viewLoginButton <|
              Maybe.withDefault [] lModel.authMethods
          )

filterTeams : String -> List Team -> List Team
filterTeams teamFilter teams =
  let
    filteredList =
      List.filter
        (teamNameContains <| String.toLower teamFilter) teams
  in let
    (startingTeams, notStartingTeams) =
      List.partition (teamNameStartsWith <| String.toLower teamFilter) filteredList
  in let
    (caseSensitive, notCaseSensitive) =
      List.partition (teamNameStartsWithSensitive teamFilter) startingTeams
  in
    caseSensitive ++ notCaseSensitive ++ notStartingTeams

teamNameContains : String -> Team -> Bool
teamNameContains substring team =
  String.contains substring <|
    String.toLower team.name

teamNameStartsWith : String -> Team -> Bool
teamNameStartsWith substring team =
  String.startsWith substring <|
    String.toLower team.name

teamNameStartsWithSensitive : String -> Team -> Bool
teamNameStartsWithSensitive substring team =
  String.startsWith substring team.name

viewTeam : Team -> Html Action
viewTeam team =
  Html.a
    [ Events.onClick <| SelectTeam team.name, Attributes.href "javascript:void(0)" ]
    [ Html.text <| team.name ]

viewLoginButton : AuthMethod -> Maybe (Html action)
viewLoginButton method =
  case method of
    BasicMethod -> Nothing
    OAuthMethod oAuthMethod ->
      Just <|
        Html.form
          [ Attributes.action oAuthMethod.authURL ]
          [ Html.button
            [ Attributes.type' "submit" ]
            [ Html.text <| "login with " ++ oAuthMethod.displayName ]
          ]
