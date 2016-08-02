port module LoginPage exposing (..)

import Html.App
import Time

import Login

main =
  Html.App.programWithFlags
    { init = Login.init
    , update = Login.update
    , view = Login.view
    , subscriptions = always Sub.none
    }
