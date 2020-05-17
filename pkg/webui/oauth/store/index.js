// Copyright © 2019 The Things Network Foundation, The Things Industries B.V.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import * as Sentry from '@sentry/browser'
import { createStore, applyMiddleware, compose } from 'redux'
import { createLogicMiddleware } from 'redux-logic'
import { routerMiddleware } from 'connected-react-router'
import createSentryMiddleware from 'redux-sentry-middleware'

import dev from '@ttn-lw/lib/dev'
import env from '@ttn-lw/lib/env'

import createRootReducer from './reducers'
import logic from './middleware'

if (env.sentryDsn)
  Sentry.init({
    dsn: env.sentryDsn,
    release: process.env.VERSION,
    normalizeDepth: 10,
  })

const composeEnhancers = (dev && window.__REDUX_DEVTOOLS_EXTENSION_COMPOSE__) || compose

export default function(history) {
  const middleware = applyMiddleware(
    createSentryMiddleware(Sentry, {
      actionTransformer: action => {
        if (action.type === 'GET_OAUTH_USER_SUCCESS_ME') {
          return {
            ...action,
            user: {
              ...action.user,
              ids: undefined,
            },
          }
        }
        return action
      },
      stateTransformer: state => {
        return {
          ...state,
          user: {
            ...state.user,
            user: {
              ...state.user.user,
              ids: undefined,
            },
          },
        }
      },
    }),
    routerMiddleware(history),
    createLogicMiddleware(logic),
  )

  const store = createStore(createRootReducer(history), composeEnhancers(middleware))
  if (dev && module.hot) {
    module.hot.accept('./reducers', () => {
      store.replaceReducer(createRootReducer(history))
    })
  }

  return store
}
