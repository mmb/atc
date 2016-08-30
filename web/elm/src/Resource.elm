module Resource exposing (Flags, init, update, view)

import Concourse
import Concourse.Resource
import Dict
import Html exposing (Html)
import Html.Attributes exposing (class)
import Html.Events exposing (onClick)
import Http
import Process
import String
import Task exposing (Task)
import Time exposing (Time)
import Redirect

type alias Model =
  { resourceIdentifier : Concourse.ResourceIdentifier
  , resource : (Maybe Concourse.Resource)
  , versionedResources : Maybe (List Concourse.VersionedResource)
  , pausedChanging : Bool
  }


type Msg
  = Noop
  | ResourceFetched (Result Http.Error Concourse.Resource)
  | TogglePaused
  | PausedToggled (Result Http.Error ())
  | VersionedResourcesFetched (Result Http.Error (List Concourse.VersionedResource))

type alias Flags =
  { teamName : String
  , pipelineName : String
  , resourceName : String
  -- , pageSince : Int
  -- , pageUntil : Int
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
      , resource = Nothing
      , versionedResources = Nothing
      , pausedChanging = False
      }
  in
    ( model
    , Cmd.batch
        [ fetchResource 0 model.resourceIdentifier
        , fetchVersionedResources 0 model.resourceIdentifier
        ]
    )

update : Msg -> Model -> (Model, Cmd Msg)
update action model =
  case action of
    Noop ->
      (model, Cmd.none)
    ResourceFetched (Ok resource) ->
      ( { model | resource = Just resource }
      , fetchResource (5 * Time.second) model.resourceIdentifier
      )
    ResourceFetched (Err err) ->
      Debug.log ("failed to fetch resource: " ++ toString err) <|
        (model, Cmd.none)
    TogglePaused ->
      Debug.log ("toggle paused ") <|
      case model.resource of
        Nothing -> (model, Cmd.none)
        Just r ->
          ( { model
            | pausedChanging = True
            , resource = Just { r | paused = not r.paused }
            }
          , if r.paused
            then unpauseResource model.resourceIdentifier
            else pauseResource model.resourceIdentifier
          )
    PausedToggled (Ok ()) ->
      ( { model | pausedChanging = False} , Cmd.none)
    PausedToggled (Err (Http.BadResponse 401 _)) ->
      (model, redirectToLogin model)
    PausedToggled (Err err) ->
      Debug.log ("failed to pause/unpause resource checking: " ++ toString err) <|
        (model, Cmd.none)
    VersionedResourcesFetched (Ok versionedResources) ->
      ( { model | versionedResources = Just versionedResources }
      , fetchVersionedResources (5 * Time.second) model.resourceIdentifier
      )
    VersionedResourcesFetched (Err err) ->
      Debug.log ("failed to fetch versioned resources: " ++ toString err) <|
        (model, Cmd.none)

view : Model -> Html Msg
view model =
  case model.resource of
    Just resource ->
      let
        (checkStatus, checkMessage) =
          if resource.failingToCheck then
            ("fr errored fa fa-fw fa-exclamation-triangle", "checking failed")
          else
            ("fr succeeded fa fa-fw fa-check", "checking successfully")

        (paused, pausedIcon) =
          if model.pausedChanging then
            ("loading", "fa-spin fa-circle-o-notch")
          else if resource.paused then
            ("enabled", "fa-play")
          else
            ("disabled", "fa-pause")

      in
        Html.div [class "with-fixed-header"]
          [ Html.div [class "fixed-header"]
            [ Html.div [class "pagination-header"]
              [ Html.div [class "pagination fr"] [ ]
              , Html.h1 [] [Html.text resource.name]
              ]
            , Html.div [class "scrollable-body"]
              [ Html.div [class "resource-check-status"]
                [ Html.div [class "build-step"]
                  [ Html.div [class "header"]
                    ( List.append
                      [ Html.span
                        ( List.append
                          [class <| "btn-pause fl " ++ paused]
                          (if not model.pausedChanging then [onClick TogglePaused] else [])
                        )
                        [ Html.i [class <| "fa fa-fw " ++ pausedIcon] []
                        ]
                      , Html.h3 [class "js-resourceStatusText"] [Html.text checkMessage]
                      , Html.i [class <| checkStatus] []
                      ]
                      ( if resource.failingToCheck then
                          [ Html.div [class "step-body"]
                            [ Html.pre [] [Html.text resource.checkError]
                            ]
                          ]
                        else
                          []
                      )
                    )
                  ]
                ]
              , ( viewVersionedResources model.versionedResources )
              ]
            ]
          ]
    Nothing ->
      Html.div [] []

viewVersionedResources : Maybe (List Concourse.VersionedResource) -> Html Msg
viewVersionedResources versionedResources =
  Html.ul [class "list list-collapsable list-enableDisable resource-versions"]
    ( case versionedResources of
      Just vr ->
        List.map viewVersionedResource vr
      Nothing ->
        []
    )

viewVersionedResource : Concourse.VersionedResource -> Html Msg
viewVersionedResource versionedResource =
  let
    liEnabled =
      if versionedResource.enabled then
        "enabled"
      else
        "disabled"
  in
    Html.li [class <| "list-collapsable-item clearfix " ++ liEnabled]
      [ Html.a [class "fl btn-power-toggle js-toggleResource fa fa-power-off mrm"] []
      , Html.div [class "js-expandable list-collapsable-title"] [Html.text <| viewVersion versionedResource.version]
      , Html.div [class "list-collapsable-content w100 clearfix phm pvs"] []
      ]

viewVersion : Concourse.Version -> String
viewVersion version =
  String.concat (List.map viewKeyValue (Dict.toList version))

viewKeyValue : (String, String) -> String
viewKeyValue (key, value) =
  (key ++ " " ++ value)

fetchResource : Time -> Concourse.ResourceIdentifier -> Cmd Msg
fetchResource delay resourceIdentifier =
  Cmd.map ResourceFetched << Task.perform Err Ok <|
    Process.sleep delay `Task.andThen` (always <| Concourse.Resource.fetchResource resourceIdentifier)

pauseResource : Concourse.ResourceIdentifier -> Cmd Msg
pauseResource resourceIdentifier =
  Cmd.map PausedToggled << Task.perform Err Ok <|
    Concourse.Resource.pause resourceIdentifier


unpauseResource : Concourse.ResourceIdentifier -> Cmd Msg
unpauseResource resourceIdentifier =
  Cmd.map PausedToggled << Task.perform Err Ok <|
    Concourse.Resource.unpause resourceIdentifier

fetchVersionedResources : Time -> Concourse.ResourceIdentifier -> Cmd Msg
fetchVersionedResources delay resourceIdentifier =
  Cmd.map VersionedResourcesFetched << Task.perform Err Ok <|
    Process.sleep delay `Task.andThen` (always <| Concourse.Resource.fetchVersionedResources resourceIdentifier)

redirectToLogin : Model -> Cmd Msg
redirectToLogin model =
  Cmd.map (always Noop) << Task.perform Err Ok <|
    Redirect.to "/login"
