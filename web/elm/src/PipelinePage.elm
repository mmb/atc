module PipelinePage exposing (..)

import Html.App

import Pipeline

main : Program Pipeline.Flags
main =
  Html.App.programWithFlags
    { init = Pipeline.init
    , update = Pipeline.update
    , view = Pipeline.view
    , subscriptions = always Sub.none
    }
