module Resource exposing (init, update, view)

type alias Flags =
  { resourceName : String
  , teamName : String
  , pipelineName : String
  , pageSince : Int
  , pageUntil : Int
  }

type Msg
  = Noop
  | JobBuildsFetched (Result Http.Error (Paginated Build))
  | JobFetched (Result Http.Error Job)
  | BuildResourcesFetched FetchedBuildResources
  | ClockTick Time
  | TogglePaused
  | PausedToggled (Result Http.Error ())

init : Flags -> (Model, Cmd Msg)
init flags =
  let
    model =
      { jobInfo =
          { name = flags.jobName
          , teamName = flags.teamName
          , pipelineName = flags.pipelineName
          }
      , job = Nothing
      , pausedChanging = False
      , buildsWithResources = Nothing
      , now = 0
      , page =
          { direction =
              if flags.pageUntil > 0 then
                Concourse.Pagination.Until flags.pageUntil
              else
                Concourse.Pagination.Since flags.pageSince
          , limit = jobBuildsPerPage
          }
      , pagination =
          { previousPage = Nothing
          , nextPage = Nothing
          }
      }
  in
    ( model
    , Cmd.batch
        [ fetchResourceVersions 0 model.jobInfo (Just model.page)
        , fetchResource 0 model.jobInfo
        ]
    )
