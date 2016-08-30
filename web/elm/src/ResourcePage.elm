module ResourcePage exposing (main)

import Html.App
import Time

import Job

main : Program Resource.Flags
main =
  Html.App.programWithFlags
    { init = Resource.init
    , update = Resource.update
    , view = Resource.view
    , subscriptions = [] --always (Time.every Time.second Job.ClockTick)
    }
